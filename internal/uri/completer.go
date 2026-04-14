package uri

import (
	"hop.top/tlc/internal/storage"
	"hop.top/uri"
)

// GetRegistry returns a registry with tlc types registered.
// dirs is optional; nil uses default directories.
func GetRegistry(s *storage.SQLiteStorage, dirs ...*TypesDirConfig) (*uri.Registry, error) {
	reg := uri.NewRegistry()
	if err := RegisterTypes(reg, s, dirs...); err != nil {
		return nil, err
	}
	return reg, nil
}
