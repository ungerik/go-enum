//go:build unix

package enums

import (
	"io/fs"
	"os"
	"syscall"
)

// chownLike gives f the owner and group of the file with info,
// as far as the process is permitted to, and reports whether
// f has the group of that file.
func chownLike(f *os.File, info fs.FileInfo) bool {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return true
	}
	// Without permission for the owner, keep at least the group
	if f.Chown(int(stat.Uid), int(stat.Gid)) != nil {
		_ = f.Chown(-1, int(stat.Gid))
	}
	fInfo, err := f.Stat()
	if err != nil {
		return false
	}
	fStat, ok := fInfo.Sys().(*syscall.Stat_t)
	return ok && fStat.Gid == stat.Gid
}
