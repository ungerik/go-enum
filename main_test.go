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
	// os.Args[0] would be relative to the working directory of the test
	executable, err := os.Executable()
	require.NoError(t, err)
	cmd := exec.Command(executable, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GO_ENUM_TEST_MAIN=1")
	out, err := cmd.CombinedOutput()
	if exitErr, ok := errors.AsType[*exec.ExitError](err); ok {
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

// Value: protects=the command checks and rewrites the package of its path argument, and without one the current directory like for //go:generate go-enum;
// fails_when=main ignores the path argument or needs one, so a CI -validate path or a go:generate run checks or rewrites the wrong directory or fails;
// why_new=TestMain_OptionAfterPath only passes "." from the package directory, and no test runs the command without -validate;
// seam=none;
// no_claim=runs the test binary as the command, not an installed go-enum binary
func TestMain_PathArgument(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "pkg")
	require.NoError(t, os.Mkdir(dir, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "status.go"), []byte("package example\n\ntype Status string //#enum\n\nconst StatusA Status = \"a\"\n"), 0644))

	// From the parent directory, which has no Go files,
	// the path argument names the package to check and rewrite
	exitCode, stderr := runGoEnum(t, parent, "-validate", "pkg")
	assert.Equal(t, 1, exitCode)
	assert.Contains(t, stderr, "missing or outdated methods of enum type Status")
	exitCode, stderr = runGoEnum(t, parent, "pkg")
	require.Equal(t, 0, exitCode, stderr)
	written, err := os.ReadFile(filepath.Join(dir, "status.go"))
	require.NoError(t, err)
	assert.Contains(t, string(written), "func (s Status) Valid() bool")

	// Without path, like //go:generate go-enum, the current directory
	// is the package, which is up to date now
	exitCode, stderr = runGoEnum(t, dir)
	require.Equal(t, 0, exitCode, stderr)
	exitCode, stderr = runGoEnum(t, dir, "-validate")
	assert.Equal(t, 0, exitCode, stderr)
	unchanged, err := os.ReadFile(filepath.Join(dir, "status.go"))
	require.NoError(t, err)
	assert.Equal(t, string(written), string(unchanged))
}
