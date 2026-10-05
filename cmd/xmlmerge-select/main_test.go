package main

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestDispatchOnlyXMLAndPreserveArguments(t *testing.T) {
	for _, name := range []string{"Config/Регистр с пробелами.xml", "Config/Регистр.XML", "Config/Module.bsl", "Config/Template.txt"} {
		o := options{base: "BASE with spaces.xml", local: "НАШ.xml", remote: "ИХ.xml", output: name, rules: "rules path.xml", araxis: "C:/Program Files/Araxis/Compare.exe"}
		exe, args := command(o, "bin")
		if filepath.Ext(name) == ".xml" || filepath.Ext(name) == ".XML" {
			if exe != filepath.Join("bin", "xmlmerge-ui.exe") {
				t.Fatal(exe)
			}
			want := []string{"merge", "--rules", o.rules, "--base", o.base, "--local", o.local, "--remote", o.remote, "--output", o.output}
			if !reflect.DeepEqual(args, want) {
				t.Fatal(args)
			}
		} else {
			if exe != o.araxis || !reflect.DeepEqual(args, []string{"/merge", "/wait", "/a2", "/3", o.local, o.base, o.remote, o.output}) {
				t.Fatal(exe, args)
			}
		}
	}
}
