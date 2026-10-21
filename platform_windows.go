package main

import (
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32     = syscall.NewLazyDLL("kernel32.dll")
	lockFileEx   = kernel32.NewProc("LockFileEx")
	unlockFileEx = kernel32.NewProc("UnlockFileEx")
	replaceFileW = kernel32.NewProc("ReplaceFileW")
	moveFileExW  = kernel32.NewProc("MoveFileExW")
)

func defaultConfigBase() (string, error) { return os.UserConfigDir() }

func openRegistry(file string) (*os.File, error) {
	name, err := syscall.UTF16PtrFromString(file)
	if err != nil {
		return nil, err
	}
	// Readers must allow replacement while retaining their original snapshot.
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE|syscall.FILE_SHARE_DELETE,
		nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		return nil, &os.PathError{Op: "open", Path: file, Err: err}
	}
	return os.NewFile(uintptr(handle), file), nil
}

func lockRegistryFile(f *os.File) error {
	var overlapped syscall.Overlapped
	result, _, err := lockFileEx.Call(f.Fd(), 2, 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
	if result == 0 {
		return os.NewSyscallError("LockFileEx", err)
	}
	return nil
}

func unlockRegistryFile(f *os.File) {
	var overlapped syscall.Overlapped
	unlockFileEx.Call(f.Fd(), 0, 1, 0, uintptr(unsafe.Pointer(&overlapped)))
}

func replaceRegistry(from, to string) error {
	source, err := syscall.UTF16PtrFromString(from)
	if err != nil {
		return err
	}
	target, err := syscall.UTF16PtrFromString(to)
	if err != nil {
		return err
	}
	// ReplaceFile preserves the existing file's security settings. Creation
	// uses a same-directory move; writers already hold the registry lock.
	result, _, callErr := replaceFileW.Call(uintptr(unsafe.Pointer(target)),
		uintptr(unsafe.Pointer(source)), 0, 0, 0, 0)
	if result != 0 {
		return nil
	}
	if callErr != syscall.ERROR_FILE_NOT_FOUND {
		return os.NewSyscallError("ReplaceFileW", callErr)
	}
	result, _, callErr = moveFileExW.Call(uintptr(unsafe.Pointer(source)),
		uintptr(unsafe.Pointer(target)), 8)
	if result == 0 {
		return os.NewSyscallError("MoveFileExW", callErr)
	}
	return nil
}
