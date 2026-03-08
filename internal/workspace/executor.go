package workspace

import (
	"context"
	"sort"
	"strings"

	"github.com/charmbracelet/log"

	"hop.top/tlc/internal/core"
)

// SourceOpener creates a ProjectSource for a registered project.
// This allows tests to inject mock sources.
type SourceOpener interface {
	Open(project core.RegisteredProject) (ProjectSource, error)
}

// QueryAcross queries tasks across multiple projects and merges results.
// Each project is opened read-only via the SourceOpener.
// Tasks from each project have their ProjectID field set.
func QueryAcross(
	ctx context.Context,
	projects []core.RegisteredProject,
	q core.Query,
	opener SourceOpener,
) ([]*core.Task, error) {
	var all []*core.Task

	for _, proj := range projects {
		source, err := opener.Open(proj)
		if err != nil {
			log.Warn("skipping project; open failed",
				"project", proj.ProjectID, "error", err)
			continue
		}

		reader, err := source.OpenReadOnly()
		if err != nil {
			_ = source.Close()
			log.Warn("skipping project; read-only open failed",
				"project", proj.ProjectID, "error", err)
			continue
		}

		perDB := q
		perDB.AllProjects = true
		perDB.Limit = 0
		perDB.Offset = 0

		tasks, err := reader.ListTasks(ctx, perDB)
		_ = source.Close()
		if err != nil {
			log.Warn("skipping project; list failed",
				"project", proj.ProjectID, "error", err)
			continue
		}

		for _, t := range tasks {
			if t.ProjectID == nil || *t.ProjectID == "" {
				pid := proj.ProjectID
				t.ProjectID = &pid
			}
		}

		all = append(all, tasks...)
	}

	sortTasks(all, q.SortBy, q.SortDirection)

	// Post-merge pagination.
	if q.Offset > 0 {
		if q.Offset >= len(all) {
			return nil, nil
		}
		all = all[q.Offset:]
	}
	if q.Limit > 0 && q.Limit < len(all) {
		all = all[:q.Limit]
	}

	return all, nil
}

// sortTasks sorts in-place by the given field and direction.
func sortTasks(tasks []*core.Task, sortBy, direction string) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	desc := strings.EqualFold(direction, "desc")

	sort.SliceStable(tasks, func(i, j int) bool {
		cmp := compareTasks(tasks[i], tasks[j], sortBy)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareTasks returns -1, 0, or 1.
func compareTasks(a, b *core.Task, field string) int {
	switch field {
	case "id":
		return strings.Compare(a.ID, b.ID)
	case "title":
		return strings.Compare(a.Title, b.Title)
	case "status":
		return strings.Compare(string(a.Status), string(b.Status))
	case "updated_at":
		if a.UpdatedAt.Before(b.UpdatedAt) {
			return -1
		}
		if a.UpdatedAt.After(b.UpdatedAt) {
			return 1
		}
		return 0
	default: // "created_at"
		if a.CreatedAt.Before(b.CreatedAt) {
			return -1
		}
		if a.CreatedAt.After(b.CreatedAt) {
			return 1
		}
		return 0
	}
}
