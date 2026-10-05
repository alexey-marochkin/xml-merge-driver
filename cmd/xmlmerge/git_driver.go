package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"xmlmerge/internal/fileutil"
	"xmlmerge/internal/gitdriver"
	"xmlmerge/internal/interactive"
	"xmlmerge/internal/rules"
)

func runGitDriver(args []string, out, errOut io.Writer) (exitCode int) {
	started := time.Now()
	f := flag.NewFlagSet("git-driver", flag.ContinueOnError)
	f.SetOutput(errOut)
	var base, local, remote, path, label, baseLabel, localLabel, rulesPath, policyPath string
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	f.StringVar(&policyPath, "policy", filepath.Join(filepath.Dir(exe), "merge-policy.xml"), "supplier-selection policy XML")
	f.StringVar(&base, "base", "", "Git %O")
	f.StringVar(&local, "local", "", "Git %A; replaced only on success")
	f.StringVar(&remote, "remote", "", "Git %B")
	f.StringVar(&path, "path", "", "Git %P; metadata, never an output path")
	f.StringVar(&path, "result", "", "legacy alias for --path")
	f.StringVar(&label, "remote-label", "", "Git %Y")
	f.StringVar(&label, "r", "", "legacy alias for --remote-label")
	f.StringVar(&baseLabel, "b", "", "legacy base label; unused")
	f.StringVar(&localLabel, "l", "", "legacy local label; unused")
	f.StringVar(&rulesPath, "rules", rules.DefaultPath(), "XML rules database")
	verbose := f.Bool("verbose", false, "explain policy decisions and report elapsed time")
	if err := f.Parse(args); err != nil {
		return 2
	}
	if f.NArg() != 0 || path == "" {
		fmt.Fprintln(errOut, "git-driver requires --path (Git %P)")
		return 2
	}
	resultText, reason := "", ""
	defer func() {
		switch exitCode {
		case 1:
			resultText = "конфликт; local не изменён"
		case 2:
			resultText, reason = "ошибка", ""
		}
		if reason != "" {
			resultText += " — " + reason
		}
		fmt.Fprintf(errOut, "[xmlmerge] %q: %s\n", path, resultText)
		if *verbose {
			fmt.Fprintf(errOut, "[xmlmerge] %q: код=%d; время=%s\n", path, exitCode, time.Since(started).Round(time.Microsecond))
		}
	}()
	trace := func(format string, args ...any) {
		if *verbose {
			fmt.Fprintf(errOut, "[xmlmerge] %q: %s\n", path, fmt.Sprintf(format, args...))
		}
	}
	trace("Политика: %q; правила XML: %q", policyPath, rulesPath)
	mergeArgs := []string{"merge", "--base", base, "--local", local, "--remote", remote, "--output", local, "--rules", rulesPath}
	o, err := interactive.Parse(mergeArgs, errOut)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	policy, err := gitdriver.Load(policyPath)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	for _, data := range []string{o.Base, o.Local, o.Remote, o.Rules} {
		if interactive.SamePath(policyPath, data) {
			fmt.Fprintln(errOut, "policy path must differ from XML and rules")
			return 2
		}
	}
	left, err := os.ReadFile(o.Local)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	right, err := os.ReadFile(o.Remote)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	decision, report, err := gitdriver.SelectWithReport(policy, path, label, left, right, *verbose)
	reason = report.Reason
	for _, step := range report.Steps {
		trace("%s", step)
	}
	if err != nil {
		fmt.Fprintln(errOut, err)
		return 2
	}
	switch decision {
	case gitdriver.KeepLocal:
		resultText = "оставлен local"
		return 0
	case gitdriver.TakeRemote:
		if err := fileutil.Write(o.Local, right); err != nil {
			fmt.Fprintln(errOut, err)
			return 2
		}
		resultText = "выбрана remote"
		return 0
	default:
		// Use the existing command in-process: no Python, shell or second EXE.
		trace("Запуск трёхстороннего XML-слияния по правилам")
		resultText = "XML-слияние выполнено"
		return run(mergeArgs, out, errOut)
	}
}
