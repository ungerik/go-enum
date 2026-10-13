//go:build unix

package enums

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRewrite_KeepsFileGroup verifies that a rewritten file keeps its group,
// like when it is written in place, although it is replaced by a new file.
func TestRewrite_KeepsFileGroup(t *testing.T) {
	groups, err := os.Getgroups()
	if err != nil || len(groups) < 2 {
		t.Skip("needs a user with at least two groups")
	}
	dir := writeFiles(t, map[string]string{"status.go": `package example

type Status string //#enum

const StatusA Status = "a"
`})
	filePath := filepath.Join(dir, "status.go")
	info, err := os.Stat(filePath)
	require.NoError(t, err)
	group := groups[0]
	if stat, ok := info.Sys().(*syscall.Stat_t); ok && int(stat.Gid) == group {
		group = groups[1]
	}
	if err := os.Chown(filePath, -1, group); err != nil {
		t.Skip("can't change the group:", err)
	}
	require.NoError(t, Rewrite(dir, nil, nil, false))
	info, err = os.Stat(filePath)
	require.NoError(t, err)
	stat, ok := info.Sys().(*syscall.Stat_t)
	require.True(t, ok)
	assert.Equal(t, group, int(stat.Gid))
}

type statInfo struct {
	fs.FileInfo
	stat *syscall.Stat_t
}

func (i statInfo) Sys() any { return i.stat }

// TestChownLike_KeepsGroupOfOtherUsersFile verifies that a rewritten file of
// another user keeps at least its group, which a group member may set, else
// the new file would get the group of the process with the original mode.
func TestChownLike_KeepsGroupOfOtherUsersFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may set every owner")
	}
	groups, err := os.Getgroups()
	require.NoError(t, err)
	group := -1
	for _, g := range groups {
		if g != os.Getegid() {
			group = g
			break
		}
	}
	if group < 0 {
		t.Skip("needs a user with at least two groups")
	}
	f, err := os.CreateTemp(t.TempDir(), "status")
	require.NoError(t, err)
	defer f.Close()
	assert.True(t, chownLike(f, statInfo{stat: &syscall.Stat_t{Uid: uint32(os.Geteuid() + 1), Gid: uint32(group)}}))
	info, err := f.Stat()
	require.NoError(t, err)
	assert.Equal(t, uint32(group), info.Sys().(*syscall.Stat_t).Gid)
}

// TestChownLike_ReportsGroupNotKept verifies that chownLike reports a group
// it can't give to the new file, so that writeFile writes the file in place
// instead of applying its group permissions to the group of the process.
func TestChownLike_ReportsGroupNotKept(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may set every group")
	}
	groups, err := os.Getgroups()
	require.NoError(t, err)
	// A group the user is no member of
	group := 1
	for slices.Contains(groups, group) || group == os.Getegid() {
		group++
	}
	f, err := os.CreateTemp(t.TempDir(), "status")
	require.NoError(t, err)
	defer f.Close()
	assert.False(t, chownLike(f, statInfo{stat: &syscall.Stat_t{Uid: uint32(os.Geteuid()), Gid: uint32(group)}}))
}
