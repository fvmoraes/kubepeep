//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package securefs

import (
	"fmt"
	"io/fs"
	"os"
	"syscall"
)

func platformValidateOwner(info os.FileInfo, directory bool) error {
	stat, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("%w: owner metadata is unavailable", ErrUnsafeObject)
	}
	if int(stat.Uid) != os.Geteuid() {
		return fmt.Errorf("%w: object is not owned by current user", ErrUnsafeObject)
	}
	if !directory && uint64(stat.Nlink) != 1 {
		return fmt.Errorf("%w: regular file has multiple links", ErrUnsafeObject)
	}
	return nil
}

func platformValidateHandle(*os.File, bool, bool) error { return nil }

func platformProtectFile(file *os.File, _ string, mode fs.FileMode) error {
	return file.Chmod(mode)
}

func platformProtectDirectory(directory *os.File, _ string, mode fs.FileMode) error {
	return directory.Chmod(mode)
}
