// XML-only dispatcher for Git Extensions' "Open in mergetool" button.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type options struct{ base, local, remote, output, rules, araxis string }

func command(o options, dir string) (string, []string) {
	if strings.EqualFold(filepath.Ext(o.output), ".xml") {
		return filepath.Join(dir, "xmlmerge-ui.exe"), []string{"merge", "--rules", o.rules, "--base", o.base, "--local", o.local, "--remote", o.remote, "--output", o.output}
	}
	return o.araxis, []string{"/merge", "/wait", "/a2", "/3", o.local, o.base, o.remote, o.output}
}
func run() int {
	var o options
	f := flag.NewFlagSet("xmlmerge-select", flag.ContinueOnError)
	f.StringVar(&o.base, "base", "", "Common ancestor")
	f.StringVar(&o.local, "local", "", "Our version")
	f.StringVar(&o.remote, "remote", "", "Incoming version")
	f.StringVar(&o.output, "output", "", "Merged file")
	f.StringVar(&o.rules, "rules", "", "XML rules")
	f.StringVar(&o.araxis, "araxis", "", "Araxis Compare executable")
	if err := f.Parse(os.Args[1:]); err != nil {
		return 2
	}
	if f.NArg() != 0 || o.base == "" || o.local == "" || o.remote == "" || o.output == "" || o.rules == "" || o.araxis == "" {
		showError(fmt.Errorf("нужны --base, --local, --remote, --output, --rules, --araxis"))
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		showError(err)
		return 2
	}
	name, args := command(o, filepath.Dir(exe))
	child := exec.Command(name, args...)
	child.Stdin = os.Stdin
	child.Stdout = os.Stdout
	child.Stderr = os.Stderr
	if err = child.Run(); err != nil {
		var status *exec.ExitError
		if errors.As(err, &status) {
			return status.ExitCode()
		}
		showError(err)
		return 2
	}
	return 0
}
func main() { os.Exit(run()) }
