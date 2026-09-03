//go:build darwin || dragonfly || freebsd || illumos || linux || netbsd || openbsd

package cmd

import (
	"os"
	"syscall"
)

func lockUsageFile(file *os.File) error {
	for {
		err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			return err
		}
	}
}
