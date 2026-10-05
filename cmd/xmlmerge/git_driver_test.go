package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSupplierDriverWithRealGit(t *testing.T) {
	policyData, err := os.ReadFile("../../merge-policy.xml")
	if err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	git := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", args...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %s: %v", args, out, err)
		}
		return strings.TrimSpace(string(out))
	}
	git("init", "-q", "-b", "main")
	git("config", "user.name", "test")
	git("config", "user.email", "test@example.invalid")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "base")
	ancestor := git("rev-parse", "HEAD")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "supplier descendant")
	git("branch", "origin1c")
	git("checkout", "-qb", "vendor/releases")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "second supplier only")
	secondSupplier := git("rev-parse", "HEAD")
	git("branch", "working-same-tip", ancestor)
	git("tag", "old-supplier", ancestor)
	git("checkout", "-qb", "working-side", ancestor)
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "working side change")
	sideCommit := git("rev-parse", "HEAD")
	git("checkout", "-q", "origin1c")
	git("-c", "commit.gpgsign=false", "merge", "--no-ff", "-m", "working into supplier", "working-side")
	git("-c", "commit.gpgsign=false", "commit", "--allow-empty", "-qm", "next supplier export")
	policyData = bytes.Replace(policyData, []byte("</supplier>"), []byte("<branch>vendor/releases</branch></supplier>"), 1)
	policyData = bytes.Replace(policyData, []byte("</supplier>"), []byte("<branch>absent-supplier</branch></supplier>"), 1)
	for _, tc := range []struct {
		name, head, label, path string
		want                    int
		remote                  bool
	}{
		{"remote ancestor of origin1c", "main", ancestor + ":old/path", "folder with spaces/Form.xml", 0, true},
		{"local supplier wins", "origin1c", "missing", "РТ_/Form.xml", 0, false},
		{"second supplier local", "vendor/releases", "missing", "Form.xml", 0, false},
		{"second supplier remote", "main", secondSupplier, "Form.xml", 0, true},
		{"supplier branch by name", "main", "origin1c", "Form.xml", 0, true},
		{"supplier full ref", "main", "refs/heads/origin1c", "Form.xml", 0, true},
		{"old supplier tag", "main", "old-supplier", "Form.xml", 0, true},
		{"working name on supplier history", "main", "working-same-tip", "Form.xml", 1, false},
		{"working full ref on supplier history", "main", "refs/heads/working-same-tip", "Form.xml", 1, false},
		{"merged working SHA is not supplier", "main", sideCommit, "Form.xml", 1, false},
		{"merged working branch is not supplier", "main", "working-side", "Form.xml", 1, false},
		{"unknown label conflicts", "main", "not-a-commit", "Form.xml", 1, false},
		{"ordinary conflict", "main", ancestor, "normal.xml", 1, false},
		{"html does not imply supplier", "main", "missing", "docs/ru.html", 1, false},
		{"our prefix does not imply remote", "main", "working-side", "РТ_/Form.xml", 1, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			git("checkout", "-q", tc.head)
			for _, verbose := range []bool{false, true} {
				dir := t.TempDir()
				path := func(n string) string { return filepath.Join(dir, n) }
				for name, value := range map[string]string{"base": `<r a="base"/>`, "local": `<r a="local"/>`, "remote": `<r a="remote"/>`} {
					if err := os.WriteFile(path(name), []byte(value), 0600); err != nil {
						t.Fatal(err)
					}
				}
				if err := os.WriteFile(path("policy.xml"), policyData, 0600); err != nil {
					t.Fatal(err)
				}
				var log, stdout bytes.Buffer
				args := []string{"git-driver", "--policy", path("policy.xml"), "--base", path("base"), "--local", path("local"), "--remote", path("remote"), "--path", tc.path, "--remote-label", tc.label, "--rules", path("rules.xml")}
				if verbose {
					args = append(args, "--verbose")
				}
				code := run(args, &stdout, &log)
				if code != tc.want {
					t.Fatalf("code %d: %s", code, log.String())
				}
				if stdout.Len() != 0 {
					t.Fatal("diagnostics leaked to stdout", stdout.String())
				}
				wantStatus := "оставлен local"
				if tc.remote {
					wantStatus = "выбрана remote"
				}
				if tc.want == 1 {
					wantStatus = "конфликт; local не изменён"
				}
				if !strings.Contains(log.String(), wantStatus) {
					t.Fatal(log.String())
				}
				if strings.Contains(log.String(), `<r a=`) {
					t.Fatal("XML leaked into log", log.String())
				}
				if verbose {
					if !strings.Contains(log.String(), "время=") || !strings.Contains(log.String(), "remote-метка:") {
						t.Fatal(log.String())
					}
					if tc.name == "second supplier remote" && !strings.Contains(log.String(), `Коммит найден в основной цепочке "refs/heads/vendor/releases"`) {
						t.Fatal(log.String())
					}
				} else if strings.Count(log.String(), "[xmlmerge]") != 1 || strings.Contains(log.String(), "remote-метка:") {
					t.Fatal(log.String())
				}
				got, _ := os.ReadFile(path("local"))
				expected := `<r a="local"/>`
				if tc.remote {
					expected = `<r a="remote"/>`
				}
				if string(got) != expected {
					t.Fatalf("got %s", got)
				}
				if _, err := os.Stat(tc.path); !os.IsNotExist(err) {
					t.Fatal("logical %P was used as output", err)
				}
			}
		})
	}
}

func TestDriverLogForEqualFilesAndErrors(t *testing.T) {
	dir := t.TempDir()
	write := func(name, data string) string {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		return p
	}
	base := write("base.xml", `<r>PRIVATE_CONTENT</r>`)
	local := write("local.xml", `<r>PRIVATE_CONTENT</r>`)
	remote := write("remote.xml", `<r>PRIVATE_CONTENT</r>`)
	policy := write("policy.xml", `<merge-policy version="1"><supplier><branch>origin1c</branch></supplier></merge-policy>`)
	args := []string{"git-driver", "--base", base, "--local", local, "--remote", remote, "--path", "folder/file.xml", "--policy", policy, "--rules", filepath.Join(dir, "rules.xml")}
	for _, verbose := range []bool{false, true} {
		callArgs := append([]string(nil), args...)
		if verbose {
			callArgs = append(callArgs, "--verbose")
		}
		var log bytes.Buffer
		if code := run(callArgs, &log, &log); code != 0 {
			t.Fatal(code, log.String())
		}
		if !strings.Contains(log.String(), "local и remote совпадают") || strings.Contains(log.String(), "PRIVATE_CONTENT") {
			t.Fatal(log.String())
		}
		if verbose && !strings.Contains(log.String(), "Проверка Git не требуется") {
			t.Fatal(log.String())
		}
	}
	if err := os.Remove(policy); err != nil {
		t.Fatal(err)
	}
	var log bytes.Buffer
	if code := run(args, &log, &log); code != 2 {
		t.Fatal(code, log.String())
	}
	if !strings.Contains(log.String(), `"folder/file.xml": ошибка`) {
		t.Fatal(log.String())
	}
	got, err := os.ReadFile(local)
	if err != nil || string(got) != `<r>PRIVATE_CONTENT</r>` {
		t.Fatal(string(got), err)
	}
}
