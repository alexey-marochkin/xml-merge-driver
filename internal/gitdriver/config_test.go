package gitdriver

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPolicy(t *testing.T) {
	p, err := Load("../../merge-policy.xml")
	if err != nil || len(p.SupplierBranches()) != 1 || p.SupplierBranches()[0] != "origin1c" || len(p.Files) != 5 || len(p.Changes) != 2 || len(p.RemotePaths) != 0 || p.EligibleFile("Config/Reports/Test/Templates/Data/Ext/Template.txt") {
		t.Fatalf("policy: %+v, %v", p, err)
	}
	for _, data := range []string{
		`<merge-policy version="2"><supplier branch="vendor"/></merge-policy>`,
		`<merge-policy version="1"><supplier/></merge-policy>`,
		`<merge-policy version="1"><supplier><branch/></supplier></merge-policy>`,
		`<merge-policy version="1"><supplier><branch>vendor</branch><branch>vendor</branch></supplier></merge-policy>`,
		`<merge-policy version="1"><supplier><branch>vendor</branch><branch>bad name</branch></supplier></merge-policy>`,
		`<merge-policy version="1"><supplier branch="vendor"/><eligible-files><file>*.xml</file></eligible-files></merge-policy>`,
		`<merge-policy version="1"><supplier branch="vendor"/><prefer-remote-paths><contains/></prefer-remote-paths></merge-policy>`,
		`<merge-policy version="1"><supplier branch="--all"/></merge-policy>`,
		`<merge-policy version="1"><supplier branch="vendor"/><eligible-changes><change/></eligible-changes></merge-policy>`,
	} {
		path := filepath.Join(t.TempDir(), "policy.xml")
		os.WriteFile(path, []byte(data), 0600)
		if _, err := Load(path); err == nil {
			t.Fatalf("accepted invalid policy: %s", data)
		}
	}
}

func TestCustomPolicyReplacesEveryLegacySetting(t *testing.T) {
	p := &Policy{Version: 1, Files: []string{"Custom.xml"}, Changes: []Change{{Attributes: []Attribute{{Name: "revision"}}}}, RemotePaths: []string{"Own/"}}
	p.Supplier.Branch = "vendor/releases"
	if err := p.Validate(); err != nil {
		t.Fatal(err)
	}
	if p.EligibleFile("Rights.xml") || !p.EligibleFile("nested/Custom.xml") {
		t.Fatal("hardcoded file list")
	}
	a, b := []byte(`<r revision="1"/>`), []byte(`<r revision="2"/>`)
	if !OnlyChanges(a, b, p.Changes) || OnlyChanges([]byte(`<r version="1"/>`), []byte(`<r version="2"/>`), p.Changes) {
		t.Fatal("hardcoded attributes")
	}
	run := func(args ...string) (string, int, error) {
		if args[0] == "symbolic-ref" {
			return "vendor/releases", 0, nil
		}
		t.Fatal(args)
		return "", 0, nil
	}
	if got, err := selectWithGit(p, "Custom.xml", "", a, b, run); got != KeepLocal || err != nil {
		t.Fatal(got, err)
	}
	run = func(args ...string) (string, int, error) { return "main", 0, nil }
	if got, _ := selectWithGit(p, "Own/Custom.xml", "", a, b, run); got != TakeRemote {
		t.Fatal(got)
	}
	if got, _ := selectWithGit(p, "РТ_/Custom.xml", "", a, b, run); got != Merge {
		t.Fatal("hardcoded path condition", got)
	}
}

func TestSupplierBranchFormats(t *testing.T) {
	for _, supplier := range []string{
		`<supplier branch="vendor/releases"/>`,
		`<supplier><branch>vendor/releases</branch></supplier>`,
		`<supplier><branch>vendor/releases</branch><branch>origin1c</branch></supplier>`,
	} {
		path := filepath.Join(t.TempDir(), "policy.xml")
		if err := os.WriteFile(path, []byte(`<merge-policy version="1">`+supplier+`</merge-policy>`), 0600); err != nil {
			t.Fatal(err)
		}
		p, err := Load(path)
		if err != nil {
			t.Fatal(err)
		}
		if p.SupplierBranches()[0] != "vendor/releases" {
			t.Fatal(p.SupplierBranches())
		}
	}
}
