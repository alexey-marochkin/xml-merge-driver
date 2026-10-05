package ui

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestPreferencesPersistAndPatchIndependentFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.xml")
	a, b := &preferenceStore{path: path}, &preferenceStore{path: path}
	p, e := a.update(PreferencesPatch{})
	if e != nil || p.Theme != "light" {
		t.Fatal(p, e)
	}
	theme := "dark"
	width := 480
	if _, e = a.update(PreferencesPatch{Theme: &theme}); e != nil {
		t.Fatal(e)
	}
	if _, e = b.update(PreferencesPatch{SidebarWidth: &width}); e != nil {
		t.Fatal(e)
	}
	p, e = (&preferenceStore{path: path}).load()
	if e != nil || p.Theme != "dark" || p.SidebarWidth != 480 {
		t.Fatal(p, e)
	}
	before, _ := os.ReadFile(path)
	invalid := "bright"
	badWidth := 10000
	for _, patch := range []PreferencesPatch{{Theme: &invalid}, {SidebarWidth: &badWidth}} {
		if _, e = a.update(patch); e == nil {
			t.Fatal("invalid preference saved")
		}
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(before, after) {
		t.Fatal("rejected setting changed file")
	}
}

func TestMalformedSettingsAreNotOverwritten(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.xml")
	input := []byte("<broken")
	if e := os.WriteFile(path, input, 0600); e != nil {
		t.Fatal(e)
	}
	if _, e := (&preferenceStore{path: path}).update(PreferencesPatch{}); e == nil {
		t.Fatal("invalid XML replaced")
	}
	after, _ := os.ReadFile(path)
	if !bytes.Equal(input, after) {
		t.Fatal("file overwritten")
	}
}
