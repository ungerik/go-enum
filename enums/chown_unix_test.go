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
// the file would be written in place without the guarantee of the temporary file.
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

// TestRewrite_GroupNotKeptWritesInPlace verifies that a file of a group the
// user is no member of is written in place, else the group permissions of its
// mode would apply to the group of the process. On macOS a file created in
// /tmp gets its group wheel, else the test is skipped.
func TestRewrite_GroupNotKeptWritesInPlace(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root may set every group")
	}
	src, err := os.CreateTemp("/tmp", "status*.go")
	if err != nil {
		t.Skip("can't create a file in /tmp:", err)
	}
	_, err = src.WriteString("package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n")
	require.NoError(t, err)
	require.NoError(t, src.Close())
	filePath := filepath.Join(t.TempDir(), "status.go")
	if err := os.Rename(src.Name(), filePath); err != nil {
		os.Remove(src.Name())
		t.Skip("can't move the file:", err)
	}
	before, err := os.Stat(filePath)
	require.NoError(t, err)
	gid := int(before.Sys().(*syscall.Stat_t).Gid)
	groups, err := os.Getgroups()
	require.NoError(t, err)
	if gid == os.Getegid() || slices.Contains(groups, gid) {
		t.Skip("needs a file of a group the user is no member of, like a /tmp file on macOS")
	}
	require.NoError(t, Rewrite(filepath.Dir(filePath), nil, nil, false))
	after, err := os.Stat(filePath)
	require.NoError(t, err)
	assert.True(t, os.SameFile(before, after), "must be written in place")
	assert.Equal(t, uint32(gid), after.Sys().(*syscall.Stat_t).Gid)
	assert.Contains(t, readFile(t, filepath.Dir(filePath), "status.go"), "func (s Status) Valid() bool")
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
