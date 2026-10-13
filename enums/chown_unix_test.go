//go:build unix

package enums

import (
	"os"
	"path/filepath"
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
