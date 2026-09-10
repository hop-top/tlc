package workspace

import (
	"context"
	"sort"
	"strings"

	"charm.land/log/v2"

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

	sortTasks(all, q.SortBy, q.SortDirection, q.PriorityOrder)

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

// priorityRank returns p's ordinal within order, and whether it is
// present. Kept local to this package: internal/core exposes the same
// lookup against the CONFIGURED vocabulary, while the workspace sorter
// must rank against whatever order its caller put on the query.
func priorityRank(p core.Priority, order []string) (int, bool) {
	if p == "" {
		return 0, false
	}
	for i, name := range order {
		if string(p) == name {
			return i, true
		}
	}
	return 0, false
}

// sortTasks sorts in-place by the given field and direction.
//
// priorityOrder is the priority vocabulary in rank order, used only for
// sortBy=="priority"; see compareTasks.
func sortTasks(tasks []*core.Task, sortBy, direction string, priorityOrder []string) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	desc := strings.EqualFold(direction, "desc")

	// Unset priority sorts last in BOTH directions, so it is decided
	// before the direction flip rather than inside compareTasks — the
	// same contract the SQL path implements with a separate leading
	// ORDER BY term. Matching the two matters: a workspace query and a
	// single-project query differing on where untriaged tasks land
	// would be a difference nobody could explain from the flags.
	if sortBy == "priority" {
		sort.SliceStable(tasks, func(i, j int) bool {
			iu := tasks[i].Priority == ""
			ju := tasks[j].Priority == ""
			return !iu && ju
		})
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		if sortBy == "priority" && (tasks[i].Priority == "" || tasks[j].Priority == "") {
			return false
		}
		cmp := compareTasks(tasks[i], tasks[j], sortBy, priorityOrder)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareTasks returns -1, 0, or 1.
func compareTasks(a, b *core.Task, field string, priorityOrder []string) int {
	switch field {
	case "id":
		return strings.Compare(a.ID, b.ID)
	case "title":
		return strings.Compare(a.Title, b.Title)
	case "status":
		return strings.Compare(string(a.Status), string(b.Status))
	case "priority":
		// Rank order, not text order: a vocabulary of
		// URGENT/NORMAL/LATER compares backwards as text. With no
		// vocabulary supplied there is no rank to read, so fall back to
		// the text compare this case previously did not even have — the
		// field used to drop through to created_at entirely.
		ra, oka := priorityRank(a.Priority, priorityOrder)
		rb, okb := priorityRank(b.Priority, priorityOrder)
		if !oka || !okb {
			return strings.Compare(string(a.Priority), string(b.Priority))
		}
		switch {
		case ra < rb:
			return -1
		case ra > rb:
			return 1
		}
		return 0
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
