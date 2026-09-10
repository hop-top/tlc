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

	sortTasks(all, q.SortBy, q.SortDirection, q.PriorityOrder, q.EffortOrder)

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

// vocabRank returns v's ordinal within order, and whether it is present.
// Kept local to this package: internal/core exposes the same lookup
// against the CONFIGURED vocabulary, while the workspace sorter must rank
// against whatever order its caller put on the query.
func vocabRank(v string, order []string) (int, bool) {
	if v == "" {
		return 0, false
	}
	for i, name := range order {
		if v == name {
			return i, true
		}
	}
	return 0, false
}

// rankedField reads the vocabulary-column value a task carries for a
// ranked sort field, or "" when the field is not one.
//
// One accessor so the unset-last pass and the rank compare cannot
// disagree about which fields are ranked or where their values live —
// the shape that let effort keep a text sort while priority got a ranked
// one.
func rankedField(t *core.Task, field string) string {
	switch field {
	case "priority":
		return string(t.Priority)
	case "effort":
		return string(t.Effort)
	default:
		return ""
	}
}

// isRankedField reports whether field sorts by a declared vocabulary's
// rank rather than by text.
func isRankedField(field string) bool {
	return field == "priority" || field == "effort"
}

// sortTasks sorts in-place by the given field and direction.
//
// priorityOrder and effortOrder are those vocabularies in rank order,
// used only for their own sortBy; see compareTasks.
func sortTasks(tasks []*core.Task, sortBy, direction string, priorityOrder, effortOrder []string) {
	if sortBy == "" {
		sortBy = "created_at"
	}
	desc := strings.EqualFold(direction, "desc")

	// An unset vocabulary value sorts last in BOTH directions, so it is
	// decided before the direction flip rather than inside compareTasks
	// — the same contract the SQL path implements with a separate
	// leading ORDER BY term. Matching the two matters: a workspace query
	// and a single-project query differing on where untriaged or
	// unestimated tasks land would be a difference nobody could explain
	// from the flags.
	if isRankedField(sortBy) {
		sort.SliceStable(tasks, func(i, j int) bool {
			iu := rankedField(tasks[i], sortBy) == ""
			ju := rankedField(tasks[j], sortBy) == ""
			return !iu && ju
		})
	}

	sort.SliceStable(tasks, func(i, j int) bool {
		if isRankedField(sortBy) &&
			(rankedField(tasks[i], sortBy) == "" || rankedField(tasks[j], sortBy) == "") {
			return false
		}
		cmp := compareTasks(tasks[i], tasks[j], sortBy, priorityOrder, effortOrder)
		if desc {
			return cmp > 0
		}
		return cmp < 0
	})
}

// compareRanked orders two vocabulary values by their declared RANK
// rather than by text: a priority vocabulary of URGENT/NORMAL/LATER
// compares backwards as text, and the built-in effort XS, S, M, L, XL
// compares to L, M, S, XL, XS.
//
// With no vocabulary supplied there is no rank to read, so both values
// fall back to a text compare — which for these fields previously
// dropped through to created_at entirely.
//
// A non-empty value the vocabulary does not declare — one written
// before the vocabulary was renamed — ranks len(order): after every
// declared value, ahead of unset. That is the SQL path's contract (see
// vocabRankOrder in internal/storage), and the two must agree; ranking
// it 0 instead would collapse it silently onto the first declared
// value, and comparing it as TEXT sorted it wherever its spelling fell,
// so a legacy CRITICAL led a P0..P3 vocabulary.
//
// Unset is NOT handled here: it is pinned last in both directions by
// the separate leading pass in sortTasks, which short-circuits before
// this compare ever sees it.
func compareRanked(va, vb string, order []string) int {
	if len(order) == 0 {
		return strings.Compare(va, vb)
	}
	ra, oka := vocabRank(va, order)
	rb, okb := vocabRank(vb, order)
	// Two unranked values share rank len(order), so text is the only
	// thing left to separate them by.
	if !oka && !okb {
		return strings.Compare(va, vb)
	}
	if !oka {
		ra = len(order)
	}
	if !okb {
		rb = len(order)
	}
	switch {
	case ra < rb:
		return -1
	case ra > rb:
		return 1
	}
	return 0
}

// compareTasks returns -1, 0, or 1.
func compareTasks(a, b *core.Task, field string, priorityOrder, effortOrder []string) int {
	switch field {
	case "id":
		return strings.Compare(a.ID, b.ID)
	case "title":
		return strings.Compare(a.Title, b.Title)
	case "status":
		return strings.Compare(string(a.Status), string(b.Status))
	case "priority", "effort":
		order := priorityOrder
		if field == "effort" {
			order = effortOrder
		}
		return compareRanked(rankedField(a, field), rankedField(b, field), order)
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
