//go:build !unix

package enums

// isNotReplaceable reports false on systems without unix bind mounts.
func isNotReplaceable(error) bool { return false }
