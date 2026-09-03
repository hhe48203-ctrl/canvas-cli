package cmd

import (
	"os"
	"syscall"
	"unsafe"
)

var usageLockFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("LockFileEx")

func lockUsageFile(file *os.File) error {
	var overlapped syscall.Overlapped
	const exclusiveLock = 2
	const allBytes = ^uint32(0)
	ok, _, err := usageLockFileEx.Call(file.Fd(), exclusiveLock, 0,
		uintptr(allBytes), uintptr(allBytes), uintptr(unsafe.Pointer(&overlapped)))
	if ok == 0 {
		return os.NewSyscallError("LockFileEx", err)
	}
	return nil
}
