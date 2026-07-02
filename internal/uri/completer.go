package uri

import (
	"hop.top/cite/scheme"
	"hop.top/tlc/internal/storage"
)

// GetRegistry returns a registry with tlc types registered.
// dirs is optional; nil uses default directories.
func GetRegistry(s *storage.SQLiteStorage, dirs ...*TypesDirConfig) (*scheme.Registry, error) {
	reg := scheme.NewRegistry()
	if err := RegisterTypes(reg, s, dirs...); err != nil {
		return nil, err
	}
	return reg, nil
}
