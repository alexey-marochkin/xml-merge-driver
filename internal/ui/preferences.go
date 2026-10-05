package ui

import (
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"xmlmerge/internal/fileutil"
)

type Preferences struct {
	XMLName      xml.Name `xml:"settings" json:"-"`
	Version      int      `xml:"version,attr" json:"-"`
	Theme        string   `xml:"theme"`
	SidebarWidth int      `xml:"sidebar-width"`
}

type PreferencesPatch struct {
	Theme        *string
	SidebarWidth *int
}
type preferenceStore struct {
	path string
	mu   sync.Mutex
}

func preferencesPath() (string, error) {
	dir := os.Getenv("LOCALAPPDATA")
	if dir == "" {
		var err error
		dir, err = os.UserConfigDir()
		if err != nil {
			return "", err
		}
	}
	return filepath.Join(dir, "XmlMerge", "settings.xml"), nil
}

func (s *preferenceStore) load() (Preferences, error) {
	p := Preferences{Version: 1, Theme: "light", SidebarWidth: 320}
	data, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return p, nil
	}
	if err != nil {
		return p, err
	}
	if err = xml.Unmarshal(data, &p); err != nil {
		return p, fmt.Errorf("settings.xml: %w", err)
	}
	return p, p.validate()
}

func (p Preferences) validate() error {
	if p.Version != 1 {
		return fmt.Errorf("неподдерживаемая версия settings.xml: %d", p.Version)
	}
	if p.Theme != "light" && p.Theme != "dark" {
		return fmt.Errorf("неизвестная тема: %s", p.Theme)
	}
	if p.SidebarWidth < 220 || p.SidebarWidth > 720 {
		return fmt.Errorf("ширина панели должна быть от 220 до 720")
	}
	return nil
}

// Patch only changed fields under the cross-process lock, preserving preferences
// saved by another window. Atomic replacement keeps concurrent readers safe.
func (s *preferenceStore) update(patch PreferencesPatch) (Preferences, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
		return Preferences{}, err
	}
	var lock *os.File
	var err error
	for attempt := 0; attempt < 10; attempt++ {
		lock, err = os.OpenFile(s.path+".lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if !os.IsExist(err) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		return Preferences{}, fmt.Errorf("настройки интерфейса заняты или недоступны: %w", err)
	}
	lock.Close()
	defer os.Remove(s.path + ".lock")
	p, err := s.load()
	if err != nil {
		return p, err
	}
	if patch.Theme != nil {
		p.Theme = *patch.Theme
	}
	if patch.SidebarWidth != nil {
		p.SidebarWidth = *patch.SidebarWidth
	}
	if err = p.validate(); err != nil {
		return p, err
	}
	data, err := xml.MarshalIndent(p, "", "  ")
	if err != nil {
		return p, err
	}
	err = fileutil.Write(s.path, append([]byte(xml.Header), append(data, '\n')...))
	return p, err
}
