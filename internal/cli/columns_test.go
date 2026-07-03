package cli

import (
	"testing"
)

func TestResolveColumnHeaders_CaseInsensitive(t *testing.T) {
	headers, unknown := resolveColumnHeaders([]string{"id", "Title", "STATUS"}, taskColumnHeaders)
	if len(headers) != 3 {
		t.Errorf("expected 3 headers, got %d", len(headers))
	}
	if len(unknown) != 0 {
		t.Errorf("expected no unknown keys, got %v", unknown)
	}
	if headers[0] != "ID" || headers[1] != "Title" || headers[2] != "Status" {
		t.Errorf("unexpected header values: %v", headers)
	}
}

func TestResolveColumnHeaders_SkipsUnknown(t *testing.T) {
	headers, unknown := resolveColumnHeaders([]string{"id", "bogus", "due"}, taskColumnHeaders)
	if len(headers) != 2 {
		t.Errorf("expected 2 headers, got %d", len(headers))
	}
	if len(unknown) != 1 {
		t.Errorf("expected 1 unknown key, got %d: %v", len(unknown), unknown)
	}
	if unknown[0] != "bogus" {
		t.Errorf("expected unknown key 'bogus', got %v", unknown)
	}
	if headers[0] != "ID" || headers[1] != "Due" {
		t.Errorf("unexpected header values: %v", headers)
	}
}

func TestTaskListDefaultColumns_AllResolve(t *testing.T) {
	for _, key := range taskListDefaultColumns {
		if _, ok := taskColumnHeaders[key]; !ok {
			t.Errorf("default column key %q not found in taskColumnHeaders", key)
		}
	}
}
