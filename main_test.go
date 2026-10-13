package main

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMain runs main instead of the tests when the test binary is
// started by a test as the go-enum command.
func TestMain(m *testing.M) {
	if os.Getenv("GO_ENUM_TEST_MAIN") == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func runGoEnum(t *testing.T, dir string, args ...string) (exitCode int, stderr string) {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO_ENUM_TEST_MAIN=1")
	out, err := cmd.CombinedOutput()
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), string(out)
	}
	require.NoError(t, err)
	return 0, string(out)
}

func TestMain_OptionAfterPath(t *testing.T) {
	// The flag package stops at the path, so -validate after it would be
	// taken as a second path and the files would be rewritten instead of
	// validated, which must fail with the usage exit code instead
	dir := t.TempDir()
	source := "package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte(source), 0644))

	exitCode, stderr := runGoEnum(t, dir, ".", "-validate")
	assert.Equal(t, 2, exitCode)
	assert.Contains(t, stderr, "options must come before the path")
	written, err := os.ReadFile(filepath.Join(dir, "status.go"))
	require.NoError(t, err)
	assert.Equal(t, source, string(written))

	exitCode, _ = runGoEnum(t, dir, "-validate", ".")
	assert.Equal(t, 1, exitCode, "outdated methods")
}
