//go:build !windows

package config

import (
	"fmt"
	"os"
)

func CheckConfigPermissions(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}
	perm := info.Mode().Perm()
	if perm&0077 != 0 {
		return fmt.Errorf("config file %s has permissions %o (should be 0600). Fix with: chmod 600 %s", path, perm, path)
	}
	return nil
}
