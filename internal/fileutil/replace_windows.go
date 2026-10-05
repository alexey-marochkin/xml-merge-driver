//go:build windows

package fileutil

import (
	"syscall"
	"unsafe"
)

var moveFileEx = syscall.NewLazyDLL("kernel32.dll").NewProc("MoveFileExW")

func replace(from, to string) error {
	a, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	b, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// MOVEFILE_REPLACE_EXISTING | MOVEFILE_WRITE_THROUGH
	ok, _, e := moveFileEx.Call(uintptr(unsafe.Pointer(a)), uintptr(unsafe.Pointer(b)), 9)
	if ok == 0 {
		return e
	}
	return nil
}
