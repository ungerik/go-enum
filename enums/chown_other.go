//go:build !unix

package enums

import (
	"io/fs"
	"os"
)

// chownLike does nothing on systems without unix file owners
// and reports true.
func chownLike(*os.File, fs.FileInfo) bool { return true }
