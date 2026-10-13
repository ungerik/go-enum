//go:build unix

package enums

import (
	"errors"
	"syscall"
)

// isNotReplaceable reports whether a rename error means that the target
// file can't be replaced, like a bind mounted file, but may be written.
func isNotReplaceable(err error) bool {
	return errors.Is(err, syscall.EXDEV) || errors.Is(err, syscall.EBUSY)
}
