package vtodo_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/config"
	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// customVocabulary is a five-name priority set sharing no name with the
// built-in P0..P3. Five entries rather than four so the linear spread is
// genuinely exercised rather than reproducing the built-in numbers by
// coincidence, and no overlapping names so a decoder that fell back to
// the built-ins could not accidentally pass.
func customVocabulary() []config.PriorityDefinition {
	return []config.PriorityDefinition{
		{Name: "CRITICAL"},
		{Name: "HIGH"},
		{Name: "MEDIUM"},
		{Name: "LOW"},
		{Name: "TRIVIAL"},
	}
}

func priorityTask(p core.Priority) *core.Task {
	return &core.Task{
		ID:        "task_01h455vb4pex5vsknk084sn02q",
		Title:     "x",
		Status:    core.StatusTodo,
		Priority:  p,
		CreatedAt: fixedTime,
	}
}

// TestPriority_DefaultVocabularySpread pins the numeric PRIORITY the
// built-in four-name vocabulary produces.
//
// These four numbers are an INTEROP contract, not merely today's output:
// they are what every external calendar client that has ever read a tlc
// export has seen. The rank-driven spread must keep reproducing them, so
// this asserts the numbers independently of the mechanism that makes
// them.
func TestPriority_DefaultVocabularySpread(t *testing.T) {
	defs := config.GetDefaultPriorities()
	want := map[core.Priority]string{
		core.PriorityP0: "PRIORITY:1",
		core.PriorityP1: "PRIORITY:3",
		core.PriorityP2: "PRIORITY:5",
		core.PriorityP3: "PRIORITY:7",
	}
	for p, line := range want {
		t.Run(string(p), func(t *testing.T) {
			cal, err := vtodo.BuildVCalendar(
				[]*core.Task{priorityTask(p)}, nil, nil,
				vtodo.WithPriorityVocabulary(defs),
			)
			require.NoError(t, err)
			out := mustSerialize(t, cal)
			require.Contains(t, out, line)
			require.Contains(t, out, "X-TLC-PRIORITY:"+string(p))
		})
	}
}

// TestPriority_RoundTripDefaultVocabulary round-trips every built-in
// priority through encode → decode.
func TestPriority_RoundTripDefaultVocabulary(t *testing.T) {
	defs := config.GetDefaultPriorities()
	for _, d := range defs {
		t.Run(d.Name, func(t *testing.T) {
			res := roundTrip(
				t,
				[]*core.Task{priorityTask(core.Priority(d.Name))}, nil, nil,
				vtodo.WithPriorityVocabulary(defs),
			)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, core.Priority(d.Name), res.Tasks[0].Priority)
		})
	}
}

// TestPriority_RoundTripCustomVocabulary is the test the hardcoded
// P0..P3 switch could not pass: a project whose priorities are named
// CRITICAL..TRIVIAL must round-trip its own names exactly, and must emit
// a numeric PRIORITY at all.
func TestPriority_RoundTripCustomVocabulary(t *testing.T) {
	defs := customVocabulary()
	// 1 + floor(rank*9/5) over five ranks.
	wantNumeric := []string{
		"PRIORITY:1", "PRIORITY:2", "PRIORITY:4", "PRIORITY:6", "PRIORITY:8",
	}
	for i, d := range defs {
		t.Run(d.Name, func(t *testing.T) {
			in := priorityTask(core.Priority(d.Name))

			cal, err := vtodo.BuildVCalendar(
				[]*core.Task{in}, nil, nil,
				vtodo.WithPriorityVocabulary(defs),
			)
			require.NoError(t, err)
			out := mustSerialize(t, cal)

			require.Contains(t, out, wantNumeric[i],
				"custom vocabulary must still get a numeric PRIORITY")
			require.Contains(t, out, "X-TLC-PRIORITY:"+d.Name)

			res, err := vtodo.ParseVCalendar(strings.NewReader(out),
				vtodo.WithPriorityVocabulary(defs))
			require.NoError(t, err)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, core.Priority(d.Name), res.Tasks[0].Priority,
				"custom priority name must survive the round trip verbatim")
		})
	}
}

// TestPriority_SpreadIsMonotonic guards the ordering property the whole
// mapping exists for: a more urgent rank must never encode to a LARGER
// PRIORITY number than a less urgent one, for any vocabulary size.
func TestPriority_SpreadIsMonotonic(t *testing.T) {
	for n := 1; n <= 12; n++ {
		t.Run(fmt.Sprintf("N=%d", n), func(t *testing.T) {
			defs := make([]config.PriorityDefinition, n)
			for i := range defs {
				defs[i] = config.PriorityDefinition{Name: fmt.Sprintf("R%d", i)}
			}
			prev := 0
			for i := range defs {
				cal, err := vtodo.BuildVCalendar(
					[]*core.Task{priorityTask(core.Priority(defs[i].Name))}, nil, nil,
					vtodo.WithPriorityVocabulary(defs),
				)
				require.NoError(t, err)
				got := extractPriority(t, mustSerialize(t, cal))
				require.GreaterOrEqual(t, got, 1, "must never emit the reserved 0")
				require.LessOrEqual(t, got, 9, "must stay inside the RFC 5545 range")
				require.GreaterOrEqual(t, got, prev, "spread must be non-decreasing in rank")
				prev = got
			}
			require.Equal(t, 1, firstRankPriority(t, defs),
				"most urgent rank must land on 1")
		})
	}
}

func firstRankPriority(t *testing.T, defs []config.PriorityDefinition) int {
	t.Helper()
	cal, err := vtodo.BuildVCalendar(
		[]*core.Task{priorityTask(core.Priority(defs[0].Name))}, nil, nil,
		vtodo.WithPriorityVocabulary(defs),
	)
	require.NoError(t, err)
	return extractPriority(t, mustSerialize(t, cal))
}

func extractPriority(t *testing.T, ics string) int {
	t.Helper()
	for _, line := range strings.Split(ics, "\r\n") {
		if strings.HasPrefix(line, "PRIORITY:") {
			var n int
			_, err := fmt.Sscanf(strings.TrimPrefix(line, "PRIORITY:"), "%d", &n)
			require.NoError(t, err)
			return n
		}
	}
	t.Fatalf("no PRIORITY line in:\n%s", ics)
	return 0
}

// TestPriority_EmptyEmitsNothing pins the distinction between "no
// priority set" and "the least urgent priority". An empty priority must
// emit neither PRIORITY nor X-TLC-PRIORITY, and must decode back to
// empty rather than to the bottom of the vocabulary.
func TestPriority_EmptyEmitsNothing(t *testing.T) {
	for _, defs := range [][]config.PriorityDefinition{
		config.GetDefaultPriorities(),
		customVocabulary(),
	} {
		cal, err := vtodo.BuildVCalendar(
			[]*core.Task{priorityTask("")}, nil, nil,
			vtodo.WithPriorityVocabulary(defs),
		)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.NotContains(t, out, "PRIORITY:")
		require.NotContains(t, out, "X-TLC-PRIORITY")

		res, err := vtodo.ParseVCalendar(strings.NewReader(out),
			vtodo.WithPriorityVocabulary(defs))
		require.NoError(t, err)
		require.Len(t, res.Tasks, 1)
		require.Equal(t, core.Priority(""), res.Tasks[0].Priority,
			"absent priority must stay empty, not become the least urgent value")
	}
}

// foreignICS renders a minimal VTODO carrying only a numeric PRIORITY —
// what a non-tlc calendar client would produce. `priority` of "" omits
// the property entirely.
func foreignICS(priority string) string {
	prop := ""
	if priority != "" {
		prop = "PRIORITY:" + priority + "\r\n"
	}
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//other//app//EN\r\n" +
		"BEGIN:VTODO\r\n" +
		"UID:task_01h455vb4pex5vsknk084sn02q@elsewhere.example\r\n" +
		"SUMMARY:imported\r\n" +
		"STATUS:NEEDS-ACTION\r\n" +
		prop +
		"END:VTODO\r\n" +
		"END:VCALENDAR\r\n"
}

// TestPriority_NumericFallbackForeignCalendar exercises the path taken
// when X-TLC-PRIORITY is absent: nearest-rank against the configured
// vocabulary.
func TestPriority_NumericFallbackForeignCalendar(t *testing.T) {
	t.Run("default vocabulary", func(t *testing.T) {
		// Encoded slots are 1,3,5,7; nearest rank wins, ties to the
		// more urgent.
		cases := map[string]core.Priority{
			"1": core.PriorityP0,
			"2": core.PriorityP0,
			"3": core.PriorityP1,
			"4": core.PriorityP1,
			"5": core.PriorityP2,
			"6": core.PriorityP2,
			"7": core.PriorityP3,
			"9": core.PriorityP3,
			"0": "",
		}
		for in, want := range cases {
			res, err := vtodo.ParseVCalendar(strings.NewReader(foreignICS(in)),
				vtodo.WithPriorityVocabulary(config.GetDefaultPriorities()))
			require.NoError(t, err)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, want, res.Tasks[0].Priority, "PRIORITY:%s", in)
		}
	})

	t.Run("custom vocabulary", func(t *testing.T) {
		defs := customVocabulary()
		// Encoded slots are 1,2,4,6,8.
		cases := map[string]core.Priority{
			"1": "CRITICAL",
			"2": "HIGH",
			"3": "HIGH",
			"4": "MEDIUM",
			"6": "LOW",
			"8": "TRIVIAL",
			"9": "TRIVIAL",
		}
		for in, want := range cases {
			res, err := vtodo.ParseVCalendar(strings.NewReader(foreignICS(in)),
				vtodo.WithPriorityVocabulary(defs))
			require.NoError(t, err)
			require.Len(t, res.Tasks, 1)
			require.Equal(t, want, res.Tasks[0].Priority, "PRIORITY:%s", in)
		}
	})

	t.Run("absent property decodes empty", func(t *testing.T) {
		res, err := vtodo.ParseVCalendar(strings.NewReader(foreignICS("")),
			vtodo.WithPriorityVocabulary(config.GetDefaultPriorities()))
		require.NoError(t, err)
		require.Len(t, res.Tasks, 1)
		require.Equal(t, core.Priority(""), res.Tasks[0].Priority)
	})
}

// TestPriority_NameWinsOverNumber proves X-TLC-PRIORITY is PREFERRED,
// not merely present: a component whose numeric value disagrees with its
// name decodes to the name.
func TestPriority_NameWinsOverNumber(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//tlc//vtodo//EN\r\n" +
		"BEGIN:VTODO\r\nUID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
		"SUMMARY:x\r\nSTATUS:NEEDS-ACTION\r\n" +
		"PRIORITY:9\r\nX-TLC-PRIORITY:P0\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics),
		vtodo.WithPriorityVocabulary(config.GetDefaultPriorities()))
	require.NoError(t, err)
	require.Equal(t, core.PriorityP0, res.Tasks[0].Priority)
}

// TestPriority_UnknownNameFallsBackToNumber covers a calendar exported
// under a vocabulary the importing project does not have: the foreign
// name must not leak through as an invalid priority, so the numeric
// value decides.
func TestPriority_UnknownNameFallsBackToNumber(t *testing.T) {
	ics := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//tlc//vtodo//EN\r\n" +
		"BEGIN:VTODO\r\nUID:task_01h455vb4pex5vsknk084sn02q@tlc.local\r\n" +
		"SUMMARY:x\r\nSTATUS:NEEDS-ACTION\r\n" +
		"PRIORITY:7\r\nX-TLC-PRIORITY:SOMEDAY\r\n" +
		"END:VTODO\r\nEND:VCALENDAR\r\n"

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics),
		vtodo.WithPriorityVocabulary(config.GetDefaultPriorities()))
	require.NoError(t, err)
	got := res.Tasks[0].Priority
	require.Equal(t, core.PriorityP3, got)
	require.True(t, core.ValidPriority(got),
		"decoded priority must belong to the importing vocabulary")
}

// TestPriority_ProvenanceRoundTrip covers the Meta-carried provenance:
// who set the priority, and which rule if it was derived.
func TestPriority_ProvenanceRoundTrip(t *testing.T) {
	t.Run("derived with rule", func(t *testing.T) {
		in := priorityTask(core.PriorityP1)
		in.Meta = map[string]interface{}{
			core.MetaPrioritySource: core.PrioritySourceDerived,
			core.MetaPriorityRule:   "due-soon",
		}
		cal, err := vtodo.BuildVCalendar([]*core.Task{in}, nil, nil)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.Contains(t, out, "X-TLC-PRIORITY-SOURCE:derived")
		require.Contains(t, out, "X-TLC-PRIORITY-RULE:due-soon")

		res, err := vtodo.ParseVCalendar(strings.NewReader(out))
		require.NoError(t, err)
		got := res.Tasks[0]
		require.Equal(t, core.PrioritySourceDerived, got.Meta[core.MetaPrioritySource])
		require.Equal(t, "due-soon", got.Meta[core.MetaPriorityRule])
		require.Equal(t, core.PrioritySourceDerived, core.PrioritySource(got))
	})

	t.Run("manual has no rule", func(t *testing.T) {
		in := priorityTask(core.PriorityP0)
		in.Meta = map[string]interface{}{
			core.MetaPrioritySource: core.PrioritySourceManual,
		}
		cal, err := vtodo.BuildVCalendar([]*core.Task{in}, nil, nil)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.Contains(t, out, "X-TLC-PRIORITY-SOURCE:manual")
		require.NotContains(t, out, "X-TLC-PRIORITY-RULE")

		res, err := vtodo.ParseVCalendar(strings.NewReader(out))
		require.NoError(t, err)
		got := res.Tasks[0]
		require.Equal(t, core.PrioritySourceManual, got.Meta[core.MetaPrioritySource])
		require.NotContains(t, got.Meta, core.MetaPriorityRule)
		require.Equal(t, core.PrioritySourceManual, core.PrioritySource(got))
	})

	t.Run("absent provenance emits nothing", func(t *testing.T) {
		cal, err := vtodo.BuildVCalendar(
			[]*core.Task{priorityTask(core.PriorityP2)}, nil, nil,
		)
		require.NoError(t, err)
		out := mustSerialize(t, cal)
		require.NotContains(t, out, "X-TLC-PRIORITY-SOURCE")
		require.NotContains(t, out, "X-TLC-PRIORITY-RULE")
	})
}
