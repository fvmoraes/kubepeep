//go:build !aix && !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris && !windows

package securefs

import (
	"io/fs"
	"os"
)

func platformValidateOwner(os.FileInfo, bool) error { return nil }

func platformValidateHandle(*os.File, bool, bool) error { return nil }

func platformProtectFile(file *os.File, _ string, mode fs.FileMode) error {
	return file.Chmod(mode)
}

func platformProtectDirectory(directory *os.File, _ string, mode fs.FileMode) error {
	return directory.Chmod(mode)
}
