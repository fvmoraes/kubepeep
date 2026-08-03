//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package securefs

import (
	"io/fs"
	"os"
)

func platformOpenRegular(path string, flag int, perm fs.FileMode) (*os.File, error) {
	return os.OpenFile(path, flag, perm)
}
