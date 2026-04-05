//go:build !darwin && !linux && !windows

package hdl

import "errors"

var ErrUnsupported = errors.New("hdl: URL scheme registration not supported on this platform")

func Register(scheme, appPath string) error {
	return ErrUnsupported
}
