package gitdriver

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/text/encoding/charmap"
	"golang.org/x/text/encoding/unicode"
)

func testPolicy() *Policy {
	p := &Policy{Version: 1, Files: []string{"Rights.xml", "Form.xml", "Template.xml", "Schedule.xml", "ru.html"}, Changes: []Change{{Attributes: []Attribute{{Name: "version"}}}, {Attributes: []Attribute{{Name: "version"}, {Name: "uuid"}}}}, RemotePaths: []string{"РТ_", "html"}}
	p.Supplier.Branch = "origin1c"
	return p
}

func TestKeepLocalFilePolicy(t *testing.T) {
	p := testPolicy()
	p.KeepLocalFiles = []string{"ConfigDumpInfo.xml"}
	decision, err := selectWithGit(p, "Config/ConfigDumpInfo.xml", "unknown", []byte(`<r local="1"/>`), []byte(`<r remote="2"/>`), func(...string) (string, int, error) {
		t.Fatal("explicit local rule should not consult Git")
		return "", 1, nil
	})
	if err != nil || decision != KeepLocal {
		t.Fatalf("got %v, %v", decision, err)
	}
	decision, err = selectWithGit(p, "Config/OtherConfigDumpInfo.xml", "unknown", []byte(`<r local="1"/>`), []byte(`<r remote="2"/>`), func(...string) (string, int, error) {
		t.Fatal("ordinary substantive file should not consult Git")
		return "", 1, nil
	})
	if err != nil || decision != Merge {
		t.Fatalf("basename matched too broadly: %v, %v", decision, err)
	}
}
func OnlyVersionChanged(a, b []byte) bool { return OnlyChanges(a, b, testPolicy().Changes) }
func TestVersionOnly(t *testing.T) {
	for _, tc := range []struct {
		name, left, right string
		want              bool
	}{
		{"version", `<r version="1"/>`, `<r version="2"/>`, true},
		{"uuid before version", `<r uuid="a" version="1"/>`, `<r uuid="b" version="2"/>`, true},
		{"nested", `<r><i version="1"/></r>`, `<r><i version="2"/></r>`, true},
		{"equal", `<r version="1"/>`, `<r version="1"/>`, false},
		{"uuid alone", `<r uuid="a"/>`, `<r uuid="b"/>`, false},
		{"two versions", `<r version="1"><i version="1"/></r>`, `<r version="2"><i version="2"/></r>`, false},
		{"text", `<r version="1">a</r>`, `<r version="2">b</r>`, false},
		{"attribute", `<r version="1" x="1"/>`, `<r version="2" x="2"/>`, false},
		{"added attribute", `<r/>`, `<r version="2"/>`, false},
		{"order", `<r version="1" uuid="a"/>`, `<r uuid="b" version="2"/>`, false},
		{"namespace", `<r xmlns:x="urn:x" x:version="1"/>`, `<r xmlns:x="urn:x" x:version="2"/>`, false},
		{"invalid", `<r version="1">`, `<r version="2">`, false},
		{"CDATA EOL", "<r version=\"1\"><![CDATA[a\r\nb]]></r>", "<r version=\"2\"><![CDATA[a\nb]]></r>", false},
		{"format EOL", "<r version=\"1\">\r\n<i/>\r\n</r>", "<r version=\"2\">\n<i/>\n</r>", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := OnlyVersionChanged([]byte(tc.left), []byte(tc.right)); got != tc.want {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
		})
	}
	for _, codec := range []struct {
		name, decl string
		encode     func([]byte) ([]byte, error)
	}{
		{"cp1251", "windows-1251", charmap.Windows1251.NewEncoder().Bytes},
		{"utf16", "UTF-16", unicode.UTF16(unicode.LittleEndian, unicode.UseBOM).NewEncoder().Bytes},
	} {
		t.Run(codec.name, func(t *testing.T) {
			a, _ := codec.encode([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="%s"?><r version="1">Привет</r>`, codec.decl)))
			b, _ := codec.encode([]byte(fmt.Sprintf(`<?xml version="1.0" encoding="%s"?><r version="2">Привет</r>`, codec.decl)))
			if !OnlyVersionChanged(a, b) {
				t.Fatal("encoded version change missed")
			}
		})
	}
}

func TestPolicyPriority(t *testing.T) {
	for _, tc := range []struct {
		name, path, head, label string
		contained, version      bool
		want                    Decision
	}{
		{"supplier wins locally", "РТ_/Form.xml", "origin1c", "supplier", true, false, KeepLocal},
		{"supplier remote", "Form.xml", "main", "abc:path", true, false, TakeRemote},
		{"protected alone is not sufficient", "Form.xml", "main", "abc", false, false, Merge},
		{"path override", "РТ_/Form.xml", "main", "unknown", false, false, TakeRemote},
		{"html override", "docs/ru.html", "main", "", false, false, TakeRemote},
		{"path requires eligibility", "РТ_/Normal.xml", "main", "", false, false, Merge},
		{"version remote", "Normal.xml", "main", "abc", true, true, TakeRemote},
		{"version local", "Normal.xml", "origin1c", "abc", false, true, KeepLocal},
		{"version non supplier", "Normal.xml", "main", "abc", false, true, Merge},
		{"temporary label", "Form.xml", "main", "Temporary merge branch 1", true, false, Merge},
		{"case sensitive", "form.xml", "origin1c", "abc", false, false, Merge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			left, right := []byte(`<r a="local"/>`), []byte(`<r a="remote"/>`)
			if tc.version {
				left, right = []byte(`<r version="1"/>`), []byte(`<r version="2"/>`)
			}
			run := func(args ...string) (string, int, error) {
				switch args[0] {
				case "symbolic-ref":
					return tc.head, 0, nil
				case "rev-parse":
					if args[1] == "--symbolic-full-name" {
						return "", 0, nil
					}
					if args[len(args)-1] != "abc^{commit}" {
						t.Fatalf("unexpected revision %v", args)
					}
					return "123456", 0, nil
				case "rev-list":
					return "123456", 0, nil
				case "for-each-ref":
					if tc.contained {
						return "refs/heads/origin1c", 0, nil
					}
					return "", 0, nil
				}
				t.Fatalf("unexpected Git %v", args)
				return "", 0, nil
			}
			got, err := selectWithGit(testPolicy(), tc.path, tc.label, left, right, run)
			if err != nil || got != tc.want {
				t.Fatalf("got %v %v, want %v", got, err, tc.want)
			}
		})
	}
}

func TestNoGitForIdenticalOrSubstantive(t *testing.T) {
	noGit := func(...string) (string, int, error) { t.Fatal("unexpected Git process"); return "", 0, nil }
	if got, _ := selectWithGit(testPolicy(), "Form.xml", "", []byte("same"), []byte("same"), noGit); got != KeepLocal {
		t.Fatal(got)
	}
	if got, _ := selectWithGit(testPolicy(), "normal.xml", "", []byte(`<r a="1"/>`), []byte(`<r a="2"/>`), noGit); got != Merge {
		t.Fatal(got)
	}
}

func TestReportIdentifiesAttributeRuleWithoutValues(t *testing.T) {
	for _, detailed := range []bool{false, true} {
		report := Report{detailed: detailed}
		run := func(args ...string) (string, int, error) {
			if args[0] != "symbolic-ref" {
				t.Fatal(args)
			}
			return "origin1c", 0, nil
		}
		decision, err := selectWithGitReport(testPolicy(), "normal.xml", "working", []byte(`<r version="SECRET_OLD" uuid="PRIVATE_OLD"/>`), []byte(`<r version="SECRET_NEW" uuid="PRIVATE_NEW"/>`), run, &report)
		if err != nil || decision != KeepLocal {
			t.Fatal(decision, err)
		}
		log := report.Reason + strings.Join(report.Steps, "\n")
		if !strings.Contains(log, "version + uuid") || !strings.Contains(log, "поставщик local") {
			t.Fatal(log)
		}
		if strings.Contains(log, "SECRET") || strings.Contains(log, "PRIVATE") {
			t.Fatal("values leaked", log)
		}
	}
}

func TestMultipleSupplierBranches(t *testing.T) {
	p := testPolicy()
	p.Supplier.Branch = ""
	p.Supplier.Branches = []string{"origin1c", "vendor/releases"}
	for _, tc := range []struct {
		name, head, refs string
		want             Decision
	}{
		{"second branch local", "vendor/releases", "", KeepLocal},
		{"second branch remote", "main", "refs/heads/vendor/releases", TakeRemote},
		{"first branch remote", "main", "refs/heads/origin1c", TakeRemote},
		{"both histories remote", "main", "refs/heads/origin1c\nrefs/heads/vendor/releases", TakeRemote},
		{"prefix is not exact match", "main", "refs/heads/vendor/releases/work", Merge},
		{"no supplier", "main", "", Merge},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			run := func(args ...string) (string, int, error) {
				calls++
				switch args[0] {
				case "symbolic-ref":
					return tc.head, 0, nil
				case "rev-parse":
					if args[1] == "--symbolic-full-name" {
						return "", 0, nil
					}
					return "abc", 0, nil
				case "rev-list":
					if args[1] != "--first-parent" {
						t.Fatal(args)
					}
					return "abc", 0, nil
				case "for-each-ref":
					if strings.Join(args[2:], ",") != "refs/heads/origin1c,refs/heads/vendor/releases" {
						t.Fatal(args)
					}
					return tc.refs, 0, nil
				}
				t.Fatal(args)
				return "", 1, nil
			}
			got, err := selectWithGit(p, "Form.xml", "abc", []byte(`<r x="1"/>`), []byte(`<r x="2"/>`), run)
			if err != nil || got != tc.want {
				t.Fatal(got, err)
			}
			wantCalls := 4
			if tc.want == TakeRemote {
				wantCalls = 5
			}
			if tc.want == KeepLocal {
				wantCalls = 1
			}
			if calls != wantCalls {
				t.Fatal("Git calls", calls, "want", wantCalls)
			}
		})
	}
}

func BenchmarkVersionOnly(b *testing.B) {
	left := []byte(`<r version="1">` + strings.Repeat(`<i id="x"><value>Текст</value></i>`, 1000) + `</r>`)
	right := []byte(strings.Replace(string(left), `version="1"`, `version="2"`, 1))
	b.SetBytes(int64(len(left) + len(right)))
	b.ReportAllocs()
	for b.Loop() {
		if !OnlyVersionChanged(left, right) {
			b.Fatal("not recognized")
		}
	}
}
