package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"hop.top/tlc/internal/core"
)

// Projector projects tasks onto a secondary representation (e.g. filesystem).
type Projector interface {
	ProjectTask(task *core.Task) error
	RemoveTask(taskID string) error
	RebuildAll(tasks []*core.Task) error
}

// FilesystemProjectorConfig holds settings for FilesystemProjector.
type FilesystemProjectorConfig struct {
	BaseDir string
	GroupBy []string
	SortBy  []string
}

// FilesystemProjector writes task JSON to a canonical directory and creates
// symlinks grouped by configurable fields.
type FilesystemProjector struct {
	cfg FilesystemProjectorConfig
}

// NewFilesystemProjector creates a new FilesystemProjector.
func NewFilesystemProjector(cfg FilesystemProjectorConfig) *FilesystemProjector {
	if cfg.BaseDir == "" {
		cfg.BaseDir = ".tlc/tasks"
	}
	if len(cfg.GroupBy) == 0 {
		cfg.GroupBy = []string{"status"}
	}
	if len(cfg.SortBy) == 0 {
		cfg.SortBy = []string{"id"}
	}
	return &FilesystemProjector{cfg: cfg}
}

// ProjectTask writes the canonical JSON file and creates symlinks.
func (p *FilesystemProjector) ProjectTask(task *core.Task) error {
	if err := p.writeCanonical(task); err != nil {
		return fmt.Errorf("projector: write canonical for %s: %w", task.ID, err)
	}

	// Remove old symlinks for this task before creating new ones
	if err := p.removeSymlinks(task.ID); err != nil {
		return fmt.Errorf("projector: remove old symlinks for %s: %w", task.ID, err)
	}

	prefix := p.sortPrefix(task)

	for _, group := range p.cfg.GroupBy {
		if err := p.createGroupSymlinks(task, group, prefix); err != nil {
			return fmt.Errorf("projector: create %s symlinks for %s: %w", group, task.ID, err)
		}
	}
	return nil
}

// RemoveTask removes the canonical file and all symlinks for a task.
func (p *FilesystemProjector) RemoveTask(taskID string) error {
	canonical := p.canonicalPath(taskID)
	if err := os.Remove(canonical); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("projector: remove canonical %s: %w", taskID, err)
	}
	return p.removeSymlinks(taskID)
}

// RebuildAll wipes the projection directory and rebuilds from scratch.
func (p *FilesystemProjector) RebuildAll(tasks []*core.Task) error {
	// Wipe everything under base dir
	if err := os.RemoveAll(p.cfg.BaseDir); err != nil {
		return fmt.Errorf("projector: wipe base dir: %w", err)
	}

	for _, task := range tasks {
		if err := p.ProjectTask(task); err != nil {
			return err
		}
	}
	return nil
}

// canonicalPath returns the path for the canonical JSON file.
func (p *FilesystemProjector) canonicalPath(taskID string) string {
	return filepath.Join(p.cfg.BaseDir, "all", taskID+".json")
}

// writeCanonical writes task JSON to the canonical location.
func (p *FilesystemProjector) writeCanonical(task *core.Task) error {
	path := p.canonicalPath(task.ID)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create canonical dir: %w", err)
	}
	data, err := json.MarshalIndent(task, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal task %s: %w", task.ID, err)
	}
	return os.WriteFile(path, data, 0o644)
}

// removeSymlinks walks group directories and removes symlinks pointing
// to the canonical file for the given task ID.
func (p *FilesystemProjector) removeSymlinks(taskID string) error {
	canonicalName := taskID + ".json"

	err := filepath.WalkDir(p.cfg.BaseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths
		}
		// Skip the "all" directory
		rel, _ := filepath.Rel(p.cfg.BaseDir, path) //nolint:errcheck // baseDir is guaranteed ancestor of path in WalkDir
		if rel == "all" || strings.HasPrefix(rel, "all"+string(filepath.Separator)) {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		// WalkDir reports symlinks via d.Type()
		if d.Type()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return nil
			}
			if filepath.Base(target) == canonicalName {
				return os.Remove(path)
			}
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("walk projection dir: %w", err)
	}
	return nil
}

// sortPrefix composes the sort prefix from sort_by config fields.
func (p *FilesystemProjector) sortPrefix(task *core.Task) string {
	parts := make([]string, 0, len(p.cfg.SortBy))
	for _, field := range p.cfg.SortBy {
		parts = append(parts, p.fieldValue(task, field))
	}
	return strings.Join(parts, "-")
}

// fieldValue extracts a string value for a task field by name.
func (p *FilesystemProjector) fieldValue(task *core.Task, field string) string {
	switch field {
	case "id":
		return task.ID
	case "priority":
		if task.Priority == "" {
			return "_unset"
		}
		return string(task.Priority)
	case "effort":
		if task.Effort == "" {
			return "_unset"
		}
		return string(task.Effort)
	case "created_at":
		return task.CreatedAt.Format("20060102-150405")
	case "updated_at":
		return task.UpdatedAt.Format("20060102-150405")
	case "title":
		return Slug(task.Title)
	default:
		return "_unset"
	}
}

// createGroupSymlinks creates symlink(s) for a single group_by field.
func (p *FilesystemProjector) createGroupSymlinks(
	task *core.Task, group, prefix string,
) error {
	values := p.groupValues(task, group)
	if len(values) == 0 {
		values = []string{"_unset"}
	}
	for _, val := range values {
		dirName := val
		if val != "_unset" {
			dirName = Slug(val)
		}
		dir := filepath.Join(p.cfg.BaseDir, "by-"+group, dirName)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create group dir %s: %w", dir, err)
		}

		// Avoid duplicate ID in filename when sort_by includes "id"
		linkName := filepath.Join(dir, prefix+"-"+task.ID+".json")
		if strings.HasSuffix(prefix, task.ID) {
			linkName = filepath.Join(dir, prefix+".json")
		}

		// Compute relative path from link directory to canonical file
		relTarget, err := filepath.Rel(dir, p.canonicalPath(task.ID))
		if err != nil {
			return fmt.Errorf("resolve symlink target: %w", err)
		}
		if err := os.Symlink(relTarget, linkName); err != nil {
			return fmt.Errorf("create symlink %s: %w", linkName, err)
		}
	}
	return nil
}

// groupValues returns the value(s) for a group_by field.
// Multi-valued fields (tags) return one entry per value (fan-out).
func (p *FilesystemProjector) groupValues(task *core.Task, group string) []string {
	switch group {
	case "status":
		return []string{string(task.Status)}
	case "tag":
		if len(task.Tags) == 0 {
			return nil
		}
		return task.Tags
	case "priority":
		if task.Priority == "" {
			return nil
		}
		return []string{string(task.Priority)}
	case "effort":
		if task.Effort == "" {
			return nil
		}
		return []string{string(task.Effort)}
	case "assigned_to":
		if task.AssignedTo == nil || *task.AssignedTo == "" {
			return nil
		}
		return []string{*task.AssignedTo}
	case "track":
		if task.TrackID == nil || *task.TrackID == "" {
			return nil
		}
		return []string{*task.TrackID}
	default:
		return nil
	}
}

// slugRe matches non-alphanumeric sequences for slug generation.
var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug converts a string to a lowercase, hyphen-separated slug.
func Slug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRe.ReplaceAllString(s, "-")
	return strings.Trim(s, "-")
}
