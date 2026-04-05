package xrr

import (
	"context"
	"fmt"
)

// FileSession dispatches record/replay/passthrough via a Cassette.
type FileSession struct {
	mode     Mode
	cassette *FileCassette
}

// NewSession creates a FileSession with the given mode and cassette.
func NewSession(mode Mode, cassette *FileCassette) *FileSession {
	return &FileSession{mode: mode, cassette: cassette}
}

// RawResponse wraps a replayed payload when the adapter type is unknown.
type RawResponse struct {
	adapterID_ string
	Payload    map[string]any
}

func (r *RawResponse) AdapterID() string { return r.adapterID_ }

// Record executes one interaction according to the session mode.
//
//   - record:      calls do(), saves req+resp to cassette, returns resp.
//   - replay:      loads from cassette, returns RawResponse; do() NOT called.
//   - passthrough: calls do(), never touches cassette.
func (s *FileSession) Record(
	_ context.Context,
	adapter Adapter,
	req Request,
	do func() (Response, error),
) (Response, error) {
	switch s.mode {
	case ModeRecord:
		return s.record(adapter, req, do)
	case ModeReplay:
		return s.replay(adapter, req)
	case ModePassthrough:
		return do()
	default:
		return nil, fmt.Errorf("xrr: unknown mode %q", s.mode)
	}
}

// Close is a no-op for FileSession.
func (s *FileSession) Close() error { return nil }

func (s *FileSession) record(adapter Adapter, req Request, do func() (Response, error)) (Response, error) {
	resp, err := do()
	if err != nil {
		return nil, err
	}
	fp, err := adapter.Fingerprint(req)
	if err != nil {
		return nil, fmt.Errorf("xrr: fingerprint: %w", err)
	}
	if err := s.cassette.Save(adapter.ID(), fp, req, resp); err != nil {
		return nil, fmt.Errorf("xrr: save: %w", err)
	}
	return resp, nil
}

func (s *FileSession) replay(adapter Adapter, req Request) (Response, error) {
	fp, err := adapter.Fingerprint(req)
	if err != nil {
		return nil, fmt.Errorf("xrr: fingerprint: %w", err)
	}
	var reqPayload, respPayload map[string]any
	if err := s.cassette.Load(adapter.ID(), fp, &reqPayload, &respPayload); err != nil {
		return nil, err // preserves ErrCassetteMiss
	}
	return &RawResponse{adapterID_: adapter.ID(), Payload: respPayload}, nil
}
