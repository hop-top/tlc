package cli

import (
	"bytes"
	"encoding/json"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"hop.top/tlc/internal/core"
)

// The defect these tests pin is a readability one with a data-integrity
// edge. `track list` rendered the full slug in the ID column, so a
// 52-character slug pushed the Title off past half the terminal:
//
//	ID                                                    Title
//	config-driven-label-templates-and-workflow-wiring     Config-driven ...
//
// The fix makes ID the short "L-NNNN" alias and moves the slug to its own
// column that the table renderer may clip to fit. The clipping is the part
// that needs pinning hardest: a slug truncated in a table is a convenience,
// the same slug truncated in JSON is corruption, because a script reading
// it would address a track that does not exist.

// seedTrackListTracks creates tracks from explicit slug/title pairs and
// returns them carrying the Seq values storage assigned. Slugs are passed
// in rather than derived: the write path clamps new slugs to
// DefaultNewTrackSlugMaxLen, but the read ceiling is MaxTrackSlugLen, and
// the long legacy slugs that motivated this change live in that gap.
func seedTrackListTracks(t *testing.T, slugTitles ...[2]string) []*core.Track {
	t.Helper()
	s, err := getStorageRaw()
	if err != nil {
		t.Fatalf("getStorageRaw: %v", err)
	}
	defer func() { _ = s.Close() }()

	// Written through storage rather than TrackService on purpose: the
	// service clamps NEW slugs to DefaultNewTrackSlugMaxLen, so it cannot
	// produce the over-long rows that already sit in real databases. The
	// storage layer is the path those rows arrived by, and it is the shape
	// the renderer has to cope with.
	out := make([]*core.Track, 0, len(slugTitles))
	for _, st := range slugTitles {
		tr := &core.Track{
			ID:     core.NewTrackID(),
			Slug:   st[0],
			Title:  st[1],
			Type:   core.TrackTypeFeature,
			Status: core.TrackStatusActive,
		}
		if err := s.CreateTrack(t.Context(), tr); err != nil {
			t.Fatalf("create track %q: %v", st[0], err)
		}
		out = append(out, tr)
	}
	return out
}

// runTrackListCmd executes `track list` with extra args and returns stdout.
func runTrackListCmd(t *testing.T, args ...string) string {
	t.Helper()
	resetTrackListFlags()
	resetTrackFlags()

	cmd := newTestCmd()
	cmd.AddCommand(TrackCmd)
	buf := new(bytes.Buffer)
	cmd.SetOut(buf)
	cmd.SetErr(buf)
	cmd.SetArgs(append([]string{"track", "list"}, args...))

	if err := cmd.Execute(); err != nil {
		t.Fatalf("track list %v failed: %v", args, err)
	}
	return buf.String()
}

// TestTrackList_IDColumnIsAlias pins the headline fix: the ID column
// carries "L-NNNN", not the slug. Asserted on the rendered row, because
// the slug also appears in its own column now — checking only that the
// alias is present somewhere would pass on the unfixed renderer too.
func TestTrackList_IDColumnIsAlias(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		tracks := seedTrackListTracks(t, [2]string{
			"config-driven-label-templates-and-workflow-wiring",
			"Config-driven label templates and workflow wiring",
		})
		alias := core.FormatTrackSeq(tracks[0].Seq)
		if alias == "" {
			t.Fatalf("storage assigned no seq to track %q", tracks[0].Slug)
		}

		out := runTrackListCmd(t)

		var row string
		for _, line := range strings.Split(out, "\n") {
			if strings.Contains(line, tracks[0].Slug) {
				row = line
				break
			}
		}
		if row == "" {
			t.Fatalf("no row for slug %q in:\n%s", tracks[0].Slug, out)
		}

		fields := strings.Fields(row)
		if len(fields) == 0 {
			t.Fatalf("empty row: %q", row)
		}
		if fields[0] != alias {
			t.Errorf(
				"ID column = %q, want %q -- the slug is still in the ID column\n%s",
				fields[0], alias, out,
			)
		}
	})
}

// TestTrackList_HasSlugColumn pins that the slug did not simply vanish:
// moving it out of ID is only acceptable because it gets its own column.
func TestTrackList_HasSlugColumn(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		tracks := seedTrackListTracks(t, [2]string{"browser-rendering", "Browser rendering"})
		out := runTrackListCmd(t)

		if !headerContains(out, "slug") {
			t.Errorf("header %v has no Slug column\n%s", headerFields(out), out)
		}
		if !strings.Contains(out, tracks[0].Slug) {
			t.Errorf("slug %q absent from output\n%s", tracks[0].Slug, out)
		}
	})
}

// TestTrackList_SlugColumnParticipatesInCols pins the --cols contract:
// the new column must be addressable by name like every other, and
// selecting it alone must not drag other columns along.
func TestTrackList_SlugColumnParticipatesInCols(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		tracks := seedTrackListTracks(t, [2]string{"browser-rendering", "Browser rendering"})
		viper.Set("cols", []string{"id", "slug"})
		defer viper.Set("cols", nil)

		out := runTrackListCmd(t)

		if strings.Contains(out, "warning: unknown column") {
			t.Fatalf("--cols rejected a column it should know:\n%s", out)
		}
		hdr := headerFields(out)
		if len(hdr) != 2 || !strings.EqualFold(hdr[0], "id") || !strings.EqualFold(hdr[1], "slug") {
			t.Errorf("header = %v, want [ID Slug]\n%s", hdr, out)
		}
		if !strings.Contains(out, tracks[0].Slug) {
			t.Errorf("slug %q absent under --cols id,slug\n%s", tracks[0].Slug, out)
		}
	})
}

// TestTrackList_JSONSlugIsNeverTruncated is the data-integrity guard.
// Truncation is a table concern; a slug arriving elided in JSON is a
// value a script cannot resolve. Uses a slug far longer than any sane
// terminal budget so a renderer-level truncation leaking into the
// structured path would be unmistakable.
func TestTrackList_JSONSlugIsNeverTruncated(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		// A legacy-length slug: longer than any column budget a narrow
		// terminal would grant it, so a truncation leaking out of the
		// table renderer into the structured path would be unmistakable.
		long := "align-repo-scaffolding-with-hop-top-github-templates-and-wiring"
		tracks := seedTrackListTracks(t, [2]string{long, "Align repo scaffolding with shared templates"})
		want := tracks[0].Slug
		if want != long || len(want) < 50 {
			t.Fatalf("seed slug %q is not the long legacy slug this test needs", want)
		}

		viper.Set("output.format", formatJSON)
		defer viper.Set("output.format", "")

		out := runTrackListCmd(t)

		var got []trackListOutput
		if err := json.Unmarshal([]byte(out), &got); err != nil {
			t.Fatalf("unmarshal %q: %v", out, err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d rows, want 1: %s", len(got), out)
		}
		if got[0].Slug != want {
			t.Errorf(
				"JSON slug = %q, want %q -- a truncated slug in JSON is data a script cannot resolve",
				got[0].Slug, want,
			)
		}
		if strings.Contains(out, "…") || strings.Contains(got[0].Slug, "...") {
			t.Errorf("ellipsis reached the structured output:\n%s", out)
		}
		if got[0].ID != tracks[0].ID {
			t.Errorf("JSON id = %q, want durable id %q", got[0].ID, tracks[0].ID)
		}
	})
}

// TestTrackList_SeqZeroRowRendersAnID pins the FormatTrackDisplay choice
// over FormatTrackAlias. A row whose Seq never got backfilled has no
// alias to render; FormatTrackAlias would hand the table an empty string
// and the user would face an ID column with a hole in it.
func TestTrackList_SeqZeroRowRendersAnID(t *testing.T) {
	legacy := &core.Track{
		ID:     "track_01legacyrowwithnoseq00",
		Slug:   "legacy-row-with-no-sequence",
		Title:  "Legacy row",
		Type:   core.TrackTypeFeature,
		Status: core.TrackStatusActive,
	}
	if legacy.Seq != 0 {
		t.Fatalf("fixture must have Seq 0, got %d", legacy.Seq)
	}

	buf := new(bytes.Buffer)
	renderTrackListTable(buf, []trackRowData{{
		Track:    legacy,
		State:    nil,
		Progress: core.TrackProgress{},
	}}, false, nil)

	out := buf.String()
	var row string
	for _, line := range strings.Split(out, "\n") {
		if strings.Contains(line, legacy.Slug) {
			row = line
			break
		}
	}
	if row == "" {
		t.Fatalf("no row rendered for the Seq-0 track:\n%s", out)
	}
	fields := strings.Fields(row)
	if len(fields) == 0 || fields[0] != legacy.ID {
		t.Errorf(
			"ID cell = %v, want %q -- a Seq-0 row must fall back to its durable id, not render blank",
			fields, legacy.ID,
		)
	}
}

// TestTrackShow_KeepsFullSlugAndShowsAlias pins the other half of the
// contract. `track list` now leads with L-NNNN, so `track show` has to
// surface the same alias or the two views disagree about what a track is
// called. The slug must survive intact: `track show` is where a user goes
// to read the value the table may have elided, so a truncation here would
// leave no view of the full slug at all.
func TestTrackShow_KeepsFullSlugAndShowsAlias(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		long := "align-repo-scaffolding-with-hop-top-github-templates-and-wiring"
		tracks := seedTrackListTracks(t, [2]string{long, "Align repo scaffolding with shared templates"})
		alias := core.FormatTrackSeq(tracks[0].Seq)
		if alias == "" {
			t.Fatalf("storage assigned no seq")
		}

		resetTrackListFlags()
		resetTrackFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", long})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show: %v", err)
		}

		out := buf.String()
		if !strings.Contains(out, long) {
			t.Errorf("full slug %q absent from track show -- nothing else displays it in full\n%s", long, out)
		}
		if strings.Contains(out, "…") {
			t.Errorf("track show elided a value it is the canonical view of:\n%s", out)
		}
		if !strings.Contains(out, alias) {
			t.Errorf("alias %q absent from track show; list and show disagree on the identifier\n%s", alias, out)
		}
	})
}

// TestTrackShow_JSONCarriesFullSlug is the structured-output half of the
// same guard: whatever the human view does, a script must get the exact
// slug back.
func TestTrackShow_JSONCarriesFullSlug(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		long := "align-repo-scaffolding-with-hop-top-github-templates-and-wiring"
		tracks := seedTrackListTracks(t, [2]string{long, "Align repo scaffolding with shared templates"})

		viper.Set("output.format", formatJSON)
		defer viper.Set("output.format", "")

		resetTrackListFlags()
		resetTrackFlags()
		cmd := newTestCmd()
		cmd.AddCommand(TrackCmd)
		buf := new(bytes.Buffer)
		cmd.SetOut(buf)
		cmd.SetErr(buf)
		cmd.SetArgs([]string{"track", "show", long})
		if err := cmd.Execute(); err != nil {
			t.Fatalf("track show -f json: %v", err)
		}

		var got trackShowOutput
		if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
			t.Fatalf("unmarshal %q: %v", buf.String(), err)
		}
		if got.Slug != long {
			t.Errorf("JSON slug = %q, want %q", got.Slug, long)
		}
		if got.ID != tracks[0].ID {
			t.Errorf("JSON id = %q, want durable id %q", got.ID, tracks[0].ID)
		}
	})
}

// TestTrackList_SlugNeverCrowdsOutOtherColumns pins the regression the
// naive fix introduces. kit/output resolves an over-wide table by DROPPING
// low-priority columns before it truncates cells. An unbounded slug column
// at default priority fits the budget all by itself alongside ID, so the
// dropper "succeeds" by deleting Title, Type, State, Progress and
// Assignee — leaving the user with strictly less than the slug-in-ID
// rendering this change exists to improve on.
//
// The contract: on a narrow terminal the Slug column yields first. It is
// the one column whose value is also reachable by another route
// (`track show`), so it is the one that can afford to go.
func TestTrackList_SlugNeverCrowdsOutOtherColumns(t *testing.T) {
	slugPriority, ok := tableTagPriority(trackTableRow{}, "Slug")
	if !ok {
		t.Fatalf("trackTableRow has no Slug column")
	}
	for _, field := range []string{"ID", "Title", "Type", "Status", "State", "Progress", "Assignee"} {
		p, ok := tableTagPriority(trackTableRow{}, field)
		if !ok {
			t.Fatalf("trackTableRow has no %s column", field)
		}
		if slugPriority >= p {
			t.Errorf(
				"Slug priority %d >= %s priority %d -- a wide slug will evict %s instead of yielding to it",
				slugPriority, field, p, field,
			)
		}
	}

	// Same contract on the --all-projects row shape, which is a separate
	// struct and so a separate chance to get the tag wrong.
	slugPriority, ok = tableTagPriority(trackTableRowWithProject{}, "Slug")
	if !ok {
		t.Fatalf("trackTableRowWithProject has no Slug column")
	}
	for _, field := range []string{"ID", "Project", "Title", "Status"} {
		p, ok := tableTagPriority(trackTableRowWithProject{}, field)
		if !ok {
			t.Fatalf("trackTableRowWithProject has no %s column", field)
		}
		if slugPriority >= p {
			t.Errorf(
				"all-projects Slug priority %d >= %s priority %d",
				slugPriority, field, p,
			)
		}
	}
}

// tableTagPriority reads the kit/output `table:"Header[,priority=N]"` tag
// for the column whose header is name. Missing priority is kit's default
// of 5.
func tableTagPriority(row any, name string) (int, bool) {
	rt := reflect.TypeOf(row)
	for i := range rt.NumField() {
		tag := rt.Field(i).Tag.Get("table")
		if tag == "" {
			continue
		}
		parts := strings.Split(tag, ",")
		if !strings.EqualFold(strings.TrimSpace(parts[0]), name) {
			continue
		}
		prio := 5
		for _, opt := range parts[1:] {
			opt = strings.TrimSpace(opt)
			if v, found := strings.CutPrefix(opt, "priority="); found {
				n, err := strconv.Atoi(v)
				if err != nil {
					return 0, false
				}
				prio = n
			}
		}
		return prio, true
	}
	return 0, false
}

// TestTrackList_AllProjectsColumnOrder pins the --all-projects header
// order. Two different mechanisms decide it — the struct field order when
// cols is nil, and the injectAfter("id", "project") key list when it is
// not — and they disagree about where Slug sits relative to Project. The
// injected list is the one that runs (--all-projects always marks the
// column set customized), so this pins the order a user actually sees.
func TestTrackList_AllProjectsColumnOrder(t *testing.T) {
	withTestLock(func() {
		_, cleanup := setupTestDir(t)
		defer cleanup()

		seedTrackListTracks(t, [2]string{"browser-rendering", "Browser rendering"})

		out := runTrackListCmd(t, "--all-projects")

		hdr := headerFields(out)
		want := []string{"ID", "Project", "Slug", "Title"}
		if len(hdr) < len(want) {
			t.Fatalf("header %v shorter than %v\n%s", hdr, want, out)
		}
		for i, w := range want {
			if !strings.EqualFold(hdr[i], w) {
				t.Errorf("header[%d] = %q, want %q (full header %v)\n%s", i, hdr[i], w, hdr, out)
			}
		}
	})
}
