package uri

import (
	"hop.top/tlc/internal/storage"
	"hop.top/uri"
)

// GetRegistry returns a registry with tlc types registered.
func GetRegistry(s *storage.SQLiteStorage) (*uri.Registry, error) {
	reg := uri.NewRegistry()
	if err := RegisterTypes(reg, s); err != nil {
		return nil, err
	}
	return reg, nil
}
