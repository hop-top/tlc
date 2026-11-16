package vtodo_test

import (
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// TestDefaultStatusVocab_WireStatusFollowsRole pins the RFC 5545 STATUS
// value of each default status. in_review and blocked carry their own
// roles and must export IN-PROCESS, backlog must export NEEDS-ACTION.
func TestDefaultStatusVocab_WireStatusFollowsRole(t *testing.T) {
	cases := []struct {
		status core.TaskStatus
		wire   string
	}{
		{"backlog", "STATUS:NEEDS-ACTION"},
		{"todo", "STATUS:NEEDS-ACTION"},
		{"in_progress", "STATUS:IN-PROCESS"},
		{"in_review", "STATUS:IN-PROCESS"},
		{"blocked", "STATUS:IN-PROCESS"},
		{"done", "STATUS:COMPLETED"},
		{"cancelled", "STATUS:CANCELLED"},
	}
	for _, tc := range cases {
		t.Run(string(tc.status), func(t *testing.T) {
			cal, err := vtodo.BuildVCalendar(
				[]*core.Task{vocabTask(tc.status)}, nil, nil,
				vtodo.WithStatusDefinitions(config.GetDefaultStatuses()),
			)
			require.NoError(t, err)
			require.Contains(t, mustSerialize(t, cal), tc.wire)
		})
	}
}
