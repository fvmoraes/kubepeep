//go:build aix || darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package securefs

import (
	"io/fs"
	"os"

	"golang.org/x/sys/unix"
)

func platformOpenRegular(path string, flag int, perm fs.FileMode) (*os.File, error) {
	descriptor, err := unix.Open(path, flag|unix.O_CLOEXEC|unix.O_NOFOLLOW, uint32(perm.Perm()))
	if err != nil {
		return nil, err
	}
	return os.NewFile(uintptr(descriptor), path), nil
}
