//go:build !windows

package cli

import (
	"os"
	"path/filepath"
	"syscall"
)

func defaultConfigBase() (string, error) {
	home, err := os.UserHomeDir()
	return filepath.Join(home, ".config"), err
}

func openRegistry(file string) (*os.File, error) { return os.Open(file) }

func lockRegistryFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
}

func unlockRegistryFile(f *os.File) {
	syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
}

func replaceRegistry(from, to string) error {
	if err := os.Rename(from, to); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(to))
	if err != nil {
		return err
	}
	defer dir.Close()
	return dir.Sync()
}
