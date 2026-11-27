package workspace

import (
	"context"
	"errors"
	"testing"
	"time"

	"hop.top/tlc/internal/core"
)

// A rejected sort key or filter field fails every project's read, and
// QueryAcross skips unreadable projects. Without an up-front check the
// request would come back as an empty listing and a clean exit.
func TestQueryAcross_RejectsInvalidQueryInsteadOfSkippingEveryProject(t *testing.T) {
	now := time.Now()
	reader := &mockReader{tasks: []*core.Task{mkTask("T-1", "a", core.StatusTodo, now)}}
	opener := &mockOpener{sources: map[string]*mockSource{"p1": {reader: reader}}}
	projects := []core.RegisteredProject{{ProjectID: "p1"}}

	for _, tc := range []struct {
		name string
		q    core.Query
		want error
	}{
		{"sort field", core.Query{SortBy: "title; SELECT 1"}, core.ErrInvalidSortField},
		{"sort direction", core.Query{SortBy: "title", SortDirection: "sideways"}, core.ErrInvalidSortDirection},
		{"filter field", core.Query{Filters: []core.FieldFilter{{Field: "title = title OR 1=1 OR title", Value: "x"}}}, core.ErrInvalidFilterField},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := QueryAcross(context.Background(), projects, tc.q, opener)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if got != nil {
				t.Errorf("returned %d rows alongside the rejection", len(got))
			}
		})
	}

	if _, err := QueryAcross(context.Background(), projects, core.Query{SortBy: "title", SortDirection: "asc"}, opener); err != nil {
		t.Errorf("valid query rejected: %v", err)
	}
}
