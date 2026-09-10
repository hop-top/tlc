//go:build !unix

package cli

import "io/fs"

// statInode has no portable inode equivalent off unix. The fingerprint
// falls back to size and modtime, which still detect a rewrite.
func statInode(info fs.FileInfo) uint64 {
	return 0
}
