package rules

import (
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"xmlmerge/internal/fileutil"
)

// Ensure creates an empty database only if absent, under the normal writer lock.
func Ensure(path string) error {
	if _, err := os.Stat(path); err == nil {
		_, err = Load(path)
		return err
	} else if !os.IsNotExist(err) {
		return err
	}
	exists := errors.New("already exists")
	err := Update(path, func(*Database) error {
		if _, err := os.Stat(path); err == nil {
			return exists
		} else if !os.IsNotExist(err) {
			return err
		}
		return nil
	})
	if errors.Is(err, exists) {
		_, err = Load(path)
	}
	return err
}

// Update holds the lock across read-modify-write; it never overwrites a stale snapshot.
func Update(path string, change func(*Database) error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("rules database is locked or unavailable: %w", err)
	}
	lock.Close()
	defer os.Remove(path + ".lock")
	db, err := Load(path)
	if err != nil {
		return err
	}
	if err = change(db); err != nil {
		return err
	}
	if err = db.Validate(); err != nil {
		return err
	}
	data, err := xml.MarshalIndent(db, "", "  ")
	if err != nil {
		return err
	}
	if old, e := os.ReadFile(path); e == nil {
		if err = fileutil.Write(path+".bak", old); err != nil {
			return err
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	return fileutil.Write(path, append([]byte(xml.Header), append(data, '\n')...))
}
