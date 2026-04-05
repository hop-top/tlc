package xrr

import (
	"context"
	"errors"
)

// Mode controls session behaviour.
type Mode string

const (
	ModeRecord      Mode = "record"
	ModeReplay      Mode = "replay"
	ModePassthrough Mode = "passthrough"
)

// ErrCassetteMiss is returned when replay finds no matching cassette file.
var ErrCassetteMiss = errors.New("xrr: cassette miss")

// Adapter intercepts one channel type.
type Adapter interface {
	ID() string
	Fingerprint(req Request) (string, error)
	Serialize(v any) ([]byte, error)
	Deserialize(data []byte, target any) error
}

// Request is an opaque adapter-defined value.
type Request interface {
	AdapterID() string
}

// Response is an opaque adapter-defined value.
type Response interface {
	AdapterID() string
}

// Cassette reads/writes interaction files.
type Cassette interface {
	Load(adapterID, fingerprint string, reqTarget, respTarget any) error
	Save(adapterID, fingerprint string, req, resp any) error
}

// Session owns the lifecycle of one record/replay run.
type Session interface {
	Record(ctx context.Context, adapter Adapter, req Request,
		do func() (Response, error)) (Response, error)
	Close() error
}
