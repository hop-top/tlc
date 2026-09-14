package sync

import (
	"fmt"
	"strings"
	"time"

	vstar "hop.top/vstar"
	"hop.top/vstar/hashing"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// viewID stands in for an empty Task.ID while building the content
// view. The exporter refuses a component without a UID, and a remote
// task handed to DetectConflict may not carry one yet; the UID is
// stripped from the view anyway, so the stand-in never reaches the hash.
const viewID = "sync-content-view"

// viewClock pins the export clock of the content view. It only ever
// backs a task carrying neither UpdatedAt nor CreatedAt, and the
// property it lands on (DTSTAMP) is stripped from the view, so the
// value is immaterial; pinning it keeps the view a pure function of
// the task.
var viewClock = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)

// volatileProps are the exported properties the content view drops:
// identity (UID, X-TLC-TASK-SEQ, X-TLC-PROJECT-ID), instants that move
// without the task's content moving (DTSTAMP and LAST-MODIFIED are
// UpdatedAt; COMPLETED falls back to UpdatedAt when no log is passed;
// CREATED differs between a task and its remote copy) and the wire hash
// itself. Applied at every nesting level, so a VALARM loses its own UID,
// DTSTAMP and hash too.
var volatileProps = map[string]bool{
	"UID":                      true,
	"DTSTAMP":                  true,
	"LAST-MODIFIED":            true,
	"CREATED":                  true,
	"COMPLETED":                true,
	hashing.XVSTARHashProperty: true,
	vtodo.XPropTaskSeq:         true,
	vtodo.XPropProjectID:       true,
}

func init() {
	core.RegisterContentHasher(ContentHash)
}

// ContentHash returns the sync content hash of t: `sha256:<hex>` over
// the canonical bytes of the task's exported VTODO with volatileProps
// removed and the last-sync bookkeeping key scrubbed from Meta.
//
// It is NOT the X-VSTAR-HASH the exporter writes. That one covers
// DTSTAMP and LAST-MODIFIED, so it moves whenever UpdatedAt does, and
// UpdatedAt moves on saves that change nothing a remote can see (a
// stale-timeout firing, an executor claim). Two tasks whose exported
// content is the same hash the same here regardless of ID, sequence,
// project, creation instant or last modification.
func ContentHash(t *core.Task) (string, error) {
	view, err := contentView(t)
	if err != nil {
		return "", err
	}
	return hashing.Component(view), nil
}

// contentView exports t alone through the public encoder and reduces
// the resulting VTODO to its content properties.
func contentView(t *core.Task) (vstar.Component, error) {
	if t == nil {
		return vstar.Component{}, fmt.Errorf("sync: content view of nil task")
	}
	scrubbed := *t
	if scrubbed.ID == "" {
		scrubbed.ID = viewID
	}
	scrubbed.Meta = withoutKey(t.Meta, core.MetaLastSyncHash)
	cal, err := vtodo.BuildVCalendar([]*core.Task{&scrubbed}, nil, nil, vtodo.WithExportTime(viewClock))
	if err != nil {
		return vstar.Component{}, fmt.Errorf("sync: content view of task %q: %w", t.ID, err)
	}
	todos := cal.Filter(vstar.CompTodo)
	if len(todos) != 1 {
		return vstar.Component{}, fmt.Errorf("sync: content view of task %q: exporter emitted %d VTODOs, want 1", t.ID, len(todos))
	}
	return stripVolatile(todos[0]), nil
}

// withoutKey returns meta without key, sharing the original map when
// the key is absent. Never mutates its input.
func withoutKey(meta map[string]interface{}, key string) map[string]interface{} {
	if _, ok := meta[key]; !ok {
		return meta
	}
	out := make(map[string]interface{}, len(meta)-1)
	for k, v := range meta {
		if k != key {
			out[k] = v
		}
	}
	return out
}

// stripVolatile copies c without volatileProps, recursing into Sub.
func stripVolatile(c vstar.Component) vstar.Component {
	out := vstar.Component{Type: c.Type, Props: make([]vstar.Property, 0, len(c.Props))}
	for _, p := range c.Props {
		if volatileProps[strings.ToUpper(p.Name)] {
			continue
		}
		out.Props = append(out.Props, p)
	}
	if len(c.Sub) > 0 {
		out.Sub = make([]vstar.Component, 0, len(c.Sub))
		for _, s := range c.Sub {
			out.Sub = append(out.Sub, stripVolatile(s))
		}
	}
	return out
}

// MarkSynced records that t's current content is what the remote holds
// as of at: LastSyncAt is set and the content hash is stored under
// core.MetaLastSyncHash, the baseline NeedsPush and DetectConflict
// compare against next time. When the hash cannot be computed the
// timestamp is still set, any stale hash is dropped so the timestamp
// fallback applies rather than a wrong baseline, and the error is
// returned for the caller to report.
func MarkSynced(t *core.Task, at time.Time) error {
	t.LastSyncAt = &at
	h, err := ContentHash(t)
	if err != nil {
		delete(t.Meta, core.MetaLastSyncHash)
		return err
	}
	if t.Meta == nil {
		t.Meta = map[string]interface{}{}
	}
	t.Meta[core.MetaLastSyncHash] = h
	return nil
}
