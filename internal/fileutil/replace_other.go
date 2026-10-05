//go:build !windows

package fileutil

import "os"

func replace(from, to string) error { return os.Rename(from, to) }
