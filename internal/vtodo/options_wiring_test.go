package vtodo_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"hop.top/tlc/internal/core"
	"hop.top/tlc/internal/vtodo"
)

// TestWithProductID_AppearsInPRODID pins the PRODID property to the
// configured product ID rather than the package default. Without
// WithProductID reaching the encoder the envelope silently carries
// `-//tlc//vtodo//EN` no matter what the user configured.
func TestWithProductID_AppearsInPRODID(t *testing.T) {
	const custom = "-//MyOrg//Calendar//EN"

	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil,
		vtodo.WithProductID(custom))
	require.NoError(t, err)

	out := mustSerialize(t, cal)
	require.Contains(t, out, "PRODID:"+custom)
	require.NotContains(t, out, vtodo.DefaultProductID)
}

// TestWithProductID_EmptyKeepsDefault documents that a blank config
// value means "unset", not "blank PRODID". An .ics with an empty PRODID
// is malformed.
func TestWithProductID_EmptyKeepsDefault(t *testing.T) {
	cal, err := vtodo.BuildVCalendar([]*core.Task{sampleTask()}, nil, nil,
		vtodo.WithProductID(""))
	require.NoError(t, err)
	require.Contains(t, mustSerialize(t, cal), "PRODID:"+vtodo.DefaultProductID)
}

// TestWithUIDDomain_AppearsInUIDs pins every minted UID to the
// configured domain: the task's own UID, the track's, the RELATED-TO
// pointer between them, and the derived VJOURNAL UID.
func TestWithUIDDomain_AppearsInUIDs(t *testing.T) {
	const domain = "example.com"

	task := sampleTask()
	trackID := "track_01h455vb4pex5vsknk084sn02r"
	task.TrackID = &trackID
	track := &core.Track{
		ID:        trackID,
		Title:     "Auth hardening",
		Status:    core.TrackStatusActive,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}
	logs := []*core.LogEntry{{
		TaskID:    task.ID,
		Timestamp: task.CreatedAt,
		By:        "alice",
		Action:    "status_change",
		Note:      "Moved to in progress",
	}}

	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, []*core.Track{track}, logs,
		vtodo.WithUIDDomain(domain), vtodo.WithIncludeLogs(true))
	require.NoError(t, err)

	out := mustSerialize(t, cal)
	require.Contains(t, out, "UID:"+task.ID+"@"+domain)
	require.Contains(t, out, "UID:"+trackID+"@"+domain)
	require.Contains(t, out, "log-"+task.ID+"-status_change-")
	// The default domain must not leak into any component.
	require.NotContains(t, out, "@"+vtodo.DefaultUIDDomain)
}

// TestWithUIDDomain_EmptyKeepsDefault — a blank uid_domain must not
// produce a trailing-"@" UID.
func TestWithUIDDomain_EmptyKeepsDefault(t *testing.T) {
	task := sampleTask()
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil,
		vtodo.WithUIDDomain(""))
	require.NoError(t, err)
	require.Contains(t, mustSerialize(t, cal), "UID:"+task.ID+"@"+vtodo.DefaultUIDDomain)
}

// TestParse_MatchingDomainStripped is the decode half of the contract:
// a UID under the configured domain is ours, so the typeid is reused as
// the entity ID and nothing is stashed as external.
func TestParse_MatchingDomainStripped(t *testing.T) {
	const domain = "example.com"

	task := sampleTask()
	res := roundTrip(t, []*core.Task{task}, nil, nil, vtodo.WithUIDDomain(domain))

	require.Len(t, res.Tasks, 1)
	got := res.Tasks[0]
	require.Equal(t, task.ID, got.ID, "configured domain should be stripped, ID reused")
	require.Nil(t, got.Meta["external_uid"], "our own UID is not an external one")
}

// TestParse_ForeignDomainMintsFreshID is the regression guard for the
// behavior the domain check must not break. A well-formed tlc typeid
// under SOMEONE ELSE'S domain is a different entity that happens to
// share our ID shape — claiming it would silently overwrite the local
// row of the same name.
func TestParse_ForeignDomainMintsFreshID(t *testing.T) {
	const foreignUID = "task_01h455vb4pex5vsknk084sn02q@someone-else.example"

	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//other//cal//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:" + foreignUID,
		"SUMMARY:Their task",
		"STATUS:NEEDS-ACTION",
		"DTSTAMP:20260502T143000Z",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics),
		vtodo.WithUIDDomain("example.com"))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)

	got := res.Tasks[0]
	require.True(t, core.IsTaskID(got.ID), "should mint a fresh typeid, got %q", got.ID)
	require.NotEqual(t, "task_01h455vb4pex5vsknk084sn02q", got.ID,
		"a foreign domain must not let a typeid claim the local row of that name")
	require.Equal(t, foreignUID, got.Meta["external_uid"])
	require.Equal(t, "Their task", got.Title)
}

// TestParse_NonTypeIDForeignUIDStillStashed — the pre-existing foreign
// UID path (no typeid shape at all) is unchanged by the domain check.
func TestParse_NonTypeIDForeignUIDStillStashed(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//other//cal//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:reminder-42@icloud.com",
		"SUMMARY:Buy milk",
		"STATUS:NEEDS-ACTION",
		"DTSTAMP:20260502T143000Z",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics),
		vtodo.WithUIDDomain("example.com"))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.True(t, core.IsTaskID(res.Tasks[0].ID))
	require.Equal(t, "reminder-42@icloud.com", res.Tasks[0].Meta["external_uid"])
}

// TestParse_DomainMismatchAcrossConfigChange is the user-visible
// consequence spelled out: a calendar exported under one domain and
// read back under another is foreign in both directions.
func TestParse_DomainMismatchAcrossConfigChange(t *testing.T) {
	task := sampleTask()
	cal, err := vtodo.BuildVCalendar([]*core.Task{task}, nil, nil,
		vtodo.WithUIDDomain("old.example"))
	require.NoError(t, err)

	res, err := vtodo.ParseVCalendar(strings.NewReader(mustSerialize(t, cal)),
		vtodo.WithUIDDomain("new.example"))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.NotEqual(t, task.ID, res.Tasks[0].ID)
	require.Equal(t, task.ID+"@old.example", res.Tasks[0].Meta["external_uid"])
}

// TestParse_BareTypeIDUIDStillMatches — a UID with no "@" at all keeps
// its historical treatment: it is its own body, so it round-trips.
func TestParse_BareTypeIDUIDStillMatches(t *testing.T) {
	ics := strings.Join([]string{
		"BEGIN:VCALENDAR",
		"VERSION:2.0",
		"PRODID:-//tlc//vtodo//EN",
		"CALSCALE:GREGORIAN",
		"METHOD:PUBLISH",
		"BEGIN:VTODO",
		"UID:task_01h455vb4pex5vsknk084sn02q",
		"SUMMARY:Domainless",
		"STATUS:NEEDS-ACTION",
		"DTSTAMP:20260502T143000Z",
		"END:VTODO",
		"END:VCALENDAR",
		"",
	}, "\r\n")

	res, err := vtodo.ParseVCalendar(strings.NewReader(ics),
		vtodo.WithUIDDomain("example.com"))
	require.NoError(t, err)
	require.Len(t, res.Tasks, 1)
	require.Equal(t, "task_01h455vb4pex5vsknk084sn02q", res.Tasks[0].ID)
	require.Nil(t, res.Tasks[0].Meta["external_uid"])
}

// TestParse_TrackDomainRoundTrip covers the track side of the domain
// check, including the RELATED-TO PARENT pointer that links a task to
// its track through a domain-suffixed UID.
func TestParse_TrackDomainRoundTrip(t *testing.T) {
	const domain = "example.com"

	trackID := "track_01h455vb4pex5vsknk084sn02r"
	task := sampleTask()
	task.TrackID = &trackID
	track := &core.Track{
		ID:        trackID,
		Title:     "Auth hardening",
		Status:    core.TrackStatusActive,
		CreatedAt: task.CreatedAt,
		UpdatedAt: task.UpdatedAt,
	}

	res := roundTrip(t, []*core.Task{task}, []*core.Track{track}, nil,
		vtodo.WithUIDDomain(domain))

	require.Len(t, res.Tracks, 1)
	require.Equal(t, trackID, res.Tracks[0].ID)
	require.Len(t, res.Tasks, 1)
	require.NotNil(t, res.Tasks[0].TrackID)
	require.Equal(t, trackID, *res.Tasks[0].TrackID)
}
