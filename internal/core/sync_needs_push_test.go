package core

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestTask_NeedsPush(t *testing.T) {
	origin := testOriginGitHub
	now := time.Now().UTC()

	tests := []struct {
		name         string
		originSystem *string
		updatedAt    time.Time
		lastSyncAt   *time.Time
		expected     bool
	}{
		{
			name:         "No origin system",
			originSystem: nil,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     false,
		},
		{
			name:         "Origin system but never synced",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   nil,
			expected:     true,
		},
		{
			name:         "Synced and not modified",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   &now,
			expected:     false,
		},
		{
			name:         "Synced and then modified",
			originSystem: &origin,
			updatedAt:    now.Add(time.Minute),
			lastSyncAt:   &now,
			expected:     true,
		},
		{
			name:         "Synced after modification",
			originSystem: &origin,
			updatedAt:    now,
			lastSyncAt:   func() *time.Time { t := now.Add(time.Minute); return &t }(),
			expected:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			task := &Task{
				OriginSystem: tt.originSystem,
				UpdatedAt:    tt.updatedAt,
				LastSyncAt:   tt.lastSyncAt,
			}
			assert.Equal(t, tt.expected, task.NeedsPush())
		})
	}
}

func TestTask_NeedsPush_EdgeCases(t *testing.T) {
	github := "github"
	jira := "jira"
	now := time.Now().UTC()

	t.Run("Empty origin system string", func(t *testing.T) {
		empty := ""
		task := &Task{
			OriginSystem: &empty,
			UpdatedAt:    now,
			LastSyncAt:   nil,
		}
		assert.False(t, task.NeedsPush(), "should not need push with empty origin system")
	})

	t.Run("Different origin systems", func(t *testing.T) {
		task := &Task{
			OriginSystem: &jira,
			UpdatedAt:    now.Add(time.Minute),
			LastSyncAt:   &now,
		}
		assert.True(t, task.NeedsPush(), "should need push for any origin system with modifications")
	})

	t.Run("Updated exactly at sync time", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime,
			LastSyncAt:   &syncTime,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated at sync time")
	})

	t.Run("Updated two milliseconds after sync", func(t *testing.T) {
		syncTime := now
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    syncTime.Add(2 * time.Millisecond),
			LastSyncAt:   &syncTime,
		}
		assert.True(t, task.NeedsPush(), "should need push when updated after sync")
	})

	t.Run("Last sync is in the future", func(t *testing.T) {
		futureSync := now.Add(1 * time.Hour)
		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    now,
			LastSyncAt:   &futureSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when last sync is in future")
	})

	t.Run("Multiple modifications with sync", func(t *testing.T) {
		secondSync := now.Add(-30 * time.Minute)
		update2 := now.Add(-10 * time.Minute)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    update2,
			LastSyncAt:   &secondSync,
		}
		assert.True(t, task.NeedsPush(), "should need push when last update is after last sync")
	})

	t.Run("Updated long before sync", func(t *testing.T) {
		oldUpdate := now.Add(-24 * time.Hour)
		recentSync := now.Add(-1 * time.Hour)

		task := &Task{
			OriginSystem: &github,
			UpdatedAt:    oldUpdate,
			LastSyncAt:   &recentSync,
		}
		assert.False(t, task.NeedsPush(), "should not need push when updated before last sync")
	})
}

// The cases above pin the timestamp FALLBACK: none of them records a
// last-sync hash, so they hold with or without a content hasher. The
// cases below pin the seam itself with a stub hasher; the real,
// exporter-backed hasher is exercised in internal/sync.

func withStubHasher(t *testing.T, fn func(*Task) (string, error)) {
	t.Helper()
	RegisterContentHasher(fn)
	t.Cleanup(func() { RegisterContentHasher(nil) })
}

func TestTask_NeedsPush_HashOutranksTimestamps(t *testing.T) {
	origin := testOriginGitHub
	now := time.Now().UTC()
	withStubHasher(t, func(task *Task) (string, error) { return "sha256:" + task.Title, nil })

	synced := func(title, recorded string, updatedAt time.Time) *Task {
		return &Task{
			Title:        title,
			OriginSystem: &origin,
			UpdatedAt:    updatedAt,
			LastSyncAt:   &now,
			Meta:         map[string]interface{}{MetaLastSyncHash: recorded},
		}
	}

	t.Run("Touched after sync, content unchanged", func(t *testing.T) {
		task := synced("same", "sha256:same", now.Add(time.Hour))
		assert.False(t, task.NeedsPush(), "hash equal: a later UpdatedAt is not a change")
	})

	t.Run("Content changed, UpdatedAt before sync", func(t *testing.T) {
		task := synced("edited", "sha256:same", now.Add(-time.Hour))
		assert.True(t, task.NeedsPush(), "hash differs: an earlier UpdatedAt does not hide it")
	})

	t.Run("No hash recorded falls back to timestamps", func(t *testing.T) {
		task := synced("edited", "", now.Add(-time.Hour))
		assert.False(t, task.NeedsPush())
		task.UpdatedAt = now.Add(time.Hour)
		assert.True(t, task.NeedsPush())
	})

	t.Run("Hasher failure falls back to timestamps", func(t *testing.T) {
		withStubHasher(t, func(*Task) (string, error) { return "", assert.AnError })
		task := synced("edited", "sha256:same", now.Add(-time.Hour))
		assert.False(t, task.NeedsPush())
		task.UpdatedAt = now.Add(time.Hour)
		assert.True(t, task.NeedsPush())
	})

	t.Run("No origin system still wins", func(t *testing.T) {
		task := synced("edited", "sha256:same", now.Add(time.Hour))
		task.OriginSystem = nil
		assert.False(t, task.NeedsPush())
	})
}

func TestTask_LastSyncHash(t *testing.T) {
	var nilTask *Task
	_, ok := nilTask.LastSyncHash()
	assert.False(t, ok)

	task := &Task{}
	_, ok = task.LastSyncHash()
	assert.False(t, ok, "no Meta")

	task.Meta = map[string]interface{}{MetaLastSyncHash: 42}
	_, ok = task.LastSyncHash()
	assert.False(t, ok, "non-string value is not a hash")

	task.Meta[MetaLastSyncHash] = ""
	_, ok = task.LastSyncHash()
	assert.False(t, ok, "empty string is not a hash")

	task.Meta[MetaLastSyncHash] = "sha256:abc"
	h, ok := task.LastSyncHash()
	assert.True(t, ok)
	assert.Equal(t, "sha256:abc", h)
}
