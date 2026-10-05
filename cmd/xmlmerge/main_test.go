package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/text/encoding/charmap"
)

func TestDriverDoesNotWriteOnConflict(t *testing.T) {
	dir := t.TempDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	local := "<?xml version='1.0'?>\r\n<r v='local'><![CDATA[a\r\nb\nc]]></r>"
	for name, value := range map[string]string{"base.xml": `<r v="base"/>`, "local.xml": local, "remote.xml": `<r v="remote"/>`} {
		if err := os.WriteFile(path(name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var log bytes.Buffer
	code := run([]string{"merge", "--base", path("base.xml"), "--local", path("local.xml"), "--remote", path("remote.xml"), "--rules", path("rules.xml")}, &log, &log)
	if code != 1 {
		t.Fatalf("code %d: %s", code, log.String())
	}
	got, err := os.ReadFile(path("local.xml"))
	if err != nil || string(got) != local {
		t.Fatal("local changed", err)
	}
}

func TestEncodingFailureDoesNotWrite(t *testing.T) {
	dir := t.TempDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	local, err := charmap.Windows1251.NewEncoder().Bytes([]byte(`<?xml version="1.0" encoding="windows-1251"?><r>Привет</r>`))
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range map[string][]byte{"base.xml": local, "local.xml": local, "remote.xml": []byte(`<?xml version="1.0" encoding="UTF-8"?><r>😀</r>`)} {
		if err := os.WriteFile(path(name), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	var log bytes.Buffer
	code := run([]string{"merge", "--base", path("base.xml"), "--local", path("local.xml"), "--remote", path("remote.xml"), "--rules", path("rules.xml")}, &log, &log)
	if code == 0 {
		t.Fatal("unrepresentable result succeeded")
	}
	got, err := os.ReadFile(path("local.xml"))
	if err != nil || !bytes.Equal(got, local) {
		t.Fatal("encoding failure modified local", err)
	}
}

func TestDriverWritesSuccessfulMerge(t *testing.T) {
	dir := t.TempDir()
	path := func(s string) string { return filepath.Join(dir, s) }
	for name, value := range map[string]string{"base.xml": `<r a="1" b="1"/>`, "local.xml": `<r a="2" b="1"/>`, "remote.xml": `<r a="1" b="2"/>`} {
		if err := os.WriteFile(path(name), []byte(value), 0600); err != nil {
			t.Fatal(err)
		}
	}
	var log bytes.Buffer
	code := run([]string{"merge", "--base", path("base.xml"), "--local", path("local.xml"), "--remote", path("remote.xml"), "--rules", path("rules.xml")}, &log, &log)
	if code != 0 {
		t.Fatalf("code %d: %s", code, log.String())
	}
	got, _ := os.ReadFile(path("local.xml"))
	if string(got) != `<r a="2" b="2"/>` {
		t.Fatalf("%s", got)
	}
}
