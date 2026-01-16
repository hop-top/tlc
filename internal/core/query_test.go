package core

import "testing"

func TestQuery_Structure(t *testing.T) {
	q := Query{
		Filters: []FieldFilter{
			{Field: "status", Operator: OpEq, Value: StatusTodo},
			{Field: "assigned_to", Operator: OpNotEq, Value: "user1"},
		},
		Search: "test",
		Limit:  10,
		Offset: 20,
		SortBy: "created_at",
		Order:  "desc",
	}

	if len(q.Filters) != 2 {
		t.Errorf("expected 2 filters, got %d", len(q.Filters))
	}
	if q.Search != "test" {
		t.Errorf("expected search test, got %s", q.Search)
	}
	if q.Limit != 10 {
		t.Errorf("expected limit 10, got %d", q.Limit)
	}
}
