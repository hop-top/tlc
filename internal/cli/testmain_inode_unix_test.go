//go:build unix

package cli

import (
	"io/fs"
	"syscall"
)

// inodeOf widens whatever integer type the platform uses for st_ino.
// It is generic because the width varies across unix targets.
func inodeOf[T ~uint32 | ~uint64](ino T) uint64 { return uint64(ino) }

// statInode returns the inode number backing info, or 0 when the platform
// stat payload is not the shape this build expects. Zero simply drops
// inode from the fingerprint; size and modtime still carry it.
func statInode(info fs.FileInfo) uint64 {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return 0
	}
	return inodeOf(st.Ino)
}
