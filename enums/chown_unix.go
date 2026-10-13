//go:build unix

package enums

import (
	"io/fs"
	"os"
	"syscall"
)

// chownLike gives f the owner and group of the file with info,
// as far as the process is permitted to.
func chownLike(f *os.File, info fs.FileInfo) {
	if stat, ok := info.Sys().(*syscall.Stat_t); ok {
		// Without permission for the owner, keep at least the group
		if f.Chown(int(stat.Uid), int(stat.Gid)) != nil {
			_ = f.Chown(-1, int(stat.Gid))
		}
	}
}
