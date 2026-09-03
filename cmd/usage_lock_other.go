//go:build !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !windows

package cmd

import (
	"errors"
	"os"
)

func lockUsageFile(*os.File) error { return errors.ErrUnsupported }
