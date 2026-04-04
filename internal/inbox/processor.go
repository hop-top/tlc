package inbox

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"hop.top/tlc/internal/core"
)

// InboxResult summarizes what happened during inbox processing.
type InboxResult struct {
	Created      []string // task IDs created
	Transitioned []string // task IDs transitioned
	Failed       []string // filenames that failed
}

// InboxProcessor processes pending inbox files.
type InboxProcessor interface {
	Process(ctx context.Context) (*InboxResult, error)
}

// FileInboxProcessor processes inbox files from a directory tree.
type FileInboxProcessor struct {
	inboxDir string
	svc      *core.TaskService
	logRepo  core.LogRepository
}

// NewFileInboxProcessor creates a new FileInboxProcessor.
func NewFileInboxProcessor(
	inboxDir string,
	svc *core.TaskService,
	logRepo core.LogRepository,
) *FileInboxProcessor {
	return &FileInboxProcessor{
		inboxDir: inboxDir,
		svc:      svc,
		logRepo:  logRepo,
	}
}

// subdirs used inside the inbox directory.
const (
	dirCreate     = "create"
	dirTransition = "transition"
	dirProcessed  = "processed"
	dirFailed     = "failed"
)

// Process scans the inbox directory and processes pending files.
// Creates are processed before transitions; files within each
// subdirectory are processed in lexicographic order.
func (p *FileInboxProcessor) Process(
	ctx context.Context,
) (*InboxResult, error) {
	if err := p.ensureDirs(); err != nil {
		return nil, fmt.Errorf("inbox: ensure dirs: %w", err)
	}

	result := &InboxResult{}

	if err := p.processCreates(ctx, result); err != nil {
		return result, fmt.Errorf("inbox: process creates: %w", err)
	}

	if err := p.processTransitions(ctx, result); err != nil {
		return result, fmt.Errorf("inbox: process transitions: %w", err)
	}

	return result, nil
}

// ensureDirs creates the four required subdirectories if missing.
func (p *FileInboxProcessor) ensureDirs() error {
	for _, sub := range []string{
		dirCreate, dirTransition, dirProcessed, dirFailed,
	} {
		dir := filepath.Join(p.inboxDir, sub)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", sub, err)
		}
	}
	return nil
}

// processCreates handles all files in the create/ subdirectory.
func (p *FileInboxProcessor) processCreates(
	ctx context.Context,
	result *InboxResult,
) error {
	dir := filepath.Join(p.inboxDir, dirCreate)
	files, err := sortedFiles(dir, []string{".json", ".md"})
	if err != nil {
		return err
	}

	for _, name := range files {
		src := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			p.fail(name, err, result)
			continue
		}

		parsed, err := p.parseCreate(name, data)
		if err != nil {
			p.fail(name, err, result)
			moveFile(src, filepath.Join(
				p.inboxDir, dirFailed, name,
			))
			continue
		}

		taskID, err := p.svc.NextTaskID(ctx, "")
		if err != nil {
			p.fail(name, err, result)
			moveFile(src, filepath.Join(
				p.inboxDir, dirFailed, name,
			))
			continue
		}

		task := buildTask(taskID, parsed)

		if err := p.svc.CreateTask(
			ctx, task, "inbox", "",
		); err != nil {
			p.fail(name, err, result)
			moveFile(src, filepath.Join(
				p.inboxDir, dirFailed, name,
			))
			continue
		}

		result.Created = append(result.Created, taskID)
		moveFile(src, filepath.Join(
			p.inboxDir, dirProcessed, name,
		))
	}

	return nil
}

// processTransitions handles all .json files in the transition/
// subdirectory.
func (p *FileInboxProcessor) processTransitions(
	ctx context.Context,
	result *InboxResult,
) error {
	dir := filepath.Join(p.inboxDir, dirTransition)
	files, err := sortedFiles(dir, []string{".json"})
	if err != nil {
		return err
	}

	for _, name := range files {
		src := filepath.Join(dir, name)
		data, err := os.ReadFile(src)
		if err != nil {
			p.fail(name, err, result)
			continue
		}

		intent, err := ParseTransitionJSON(data)
		if err != nil {
			p.fail(name, err, result)
			moveFile(src, filepath.Join(
				p.inboxDir, dirFailed, name,
			))
			continue
		}

		if err := p.svc.TransitionStatus(
			ctx,
			intent.ID,
			core.TaskStatus(intent.Status),
			intent.By,
			intent.Note,
		); err != nil {
			p.fail(name, err, result)
			moveFile(src, filepath.Join(
				p.inboxDir, dirFailed, name,
			))
			continue
		}

		result.Transitioned = append(
			result.Transitioned, intent.ID,
		)
		moveFile(src, filepath.Join(
			p.inboxDir, dirProcessed, name,
		))
	}

	return nil
}

// parseCreate dispatches to the correct parser based on extension.
func (p *FileInboxProcessor) parseCreate(
	name string,
	data []byte,
) (*ParseResult, error) {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".json":
		return ParseCreateJSON(data)
	case ".md":
		return ParseCreateMarkdown(data)
	default:
		return nil, fmt.Errorf("unsupported extension: %s", ext)
	}
}

// fail records a failure: writes an .error sidecar in failed/ and
// appends the filename to result.Failed.
func (p *FileInboxProcessor) fail(
	name string,
	err error,
	result *InboxResult,
) {
	result.Failed = append(result.Failed, name)
	sidecar := filepath.Join(
		p.inboxDir, dirFailed, name+".error",
	)
	_ = os.WriteFile(sidecar, []byte(err.Error()), 0o644)
}

// buildTask converts a ParseResult into a core.Task.
func buildTask(id string, pr *ParseResult) *core.Task {
	now := time.Now().UTC()
	task := &core.Task{
		ID:          id,
		Title:       pr.Title,
		Description: pr.Description,
		Status:      core.TaskStatus(pr.Status),
		Tags:        pr.Tags,
		Effort:      core.Effort(pr.Effort),
		Priority:    core.Priority(pr.Priority),
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if pr.AssignedTo != "" {
		a := pr.AssignedTo
		task.AssignedTo = &a
	}
	if pr.TrackID != "" {
		t := pr.TrackID
		task.TrackID = &t
	}
	return task
}

// sortedFiles returns filenames from dir that match any of the given
// extensions, sorted lexicographically.
func sortedFiles(
	dir string,
	exts []string,
) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dir %s: %w", dir, err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		ext := strings.ToLower(filepath.Ext(e.Name()))
		for _, want := range exts {
			if ext == want {
				names = append(names, e.Name())
				break
			}
		}
	}
	sort.Strings(names)
	return names, nil
}

// moveFile renames src to dst, creating parent dirs as needed.
func moveFile(src, dst string) {
	_ = os.MkdirAll(filepath.Dir(dst), 0o755)
	_ = os.Rename(src, dst)
}
