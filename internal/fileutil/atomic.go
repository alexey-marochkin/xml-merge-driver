package fileutil

import (
	"os"
	"path/filepath"
)

// Write replaces a file only after the complete new contents have been flushed.
func Write(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".xmlmerge-*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp)
	if info, e := os.Stat(path); e == nil {
		if e = f.Chmod(info.Mode()); e != nil {
			f.Close()
			return e
		}
	}
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return replace(tmp, path)
}
