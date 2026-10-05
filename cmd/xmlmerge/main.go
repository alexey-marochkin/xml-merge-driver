package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"xmlmerge/internal/fileutil"
	"xmlmerge/internal/interactive"
	"xmlmerge/internal/merge"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func run(args []string, out, errOut io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(errOut, "Usage: xmlmerge inspect FILE | roundtrip INPUT OUTPUT | rules validate [FILE] | merge --base FILE --local FILE --remote FILE [--rules FILE] | git-driver --base FILE --local FILE --remote FILE --path PATH --policy FILE [--remote-label LABEL] [--rules FILE] [--verbose]")
		return 2
	}
	fail := func(err error) int { fmt.Fprintln(errOut, err); return 2 }
	switch args[0] {
	case "git-driver":
		return runGitDriver(args[1:], out, errOut)
	case "ui", "mergetool", "compare":
		uiArgs := args[1:]
		if args[0] == "mergetool" {
			uiArgs = append([]string{"merge"}, uiArgs...)
		}
		if args[0] == "compare" {
			uiArgs = append([]string{"compare"}, uiArgs...)
		}
		o, err := interactive.Parse(uiArgs, errOut)
		if err != nil {
			return fail(err)
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		return interactive.Launch(ctx, o, "", out, errOut)
	case "inspect":
		if len(args) != 2 {
			return fail(fmt.Errorf("inspect requires one XML file"))
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return fail(err)
		}
		doc, err := xmltree.Parse(data)
		if err != nil {
			return fail(err)
		}
		count, depth := 0, 0
		var visit func(*xmltree.Node, int)
		visit = func(n *xmltree.Node, d int) {
			count++
			if d > depth {
				depth = d
			}
			for _, c := range n.Children() {
				visit(c, d+1)
			}
		}
		visit(doc.Root, 1)
		info := struct {
			Root, Encoding         string
			BOM                    bool
			Bytes, Elements, Depth int
		}{doc.Root.Name.String(), doc.Format.Encoding, len(doc.Format.BOM) > 0, len(data), count, depth}
		enc := json.NewEncoder(out)
		enc.SetIndent("", "  ")
		if err := enc.Encode(info); err != nil {
			return fail(err)
		}
		return 0
	case "roundtrip":
		if len(args) != 3 {
			return fail(fmt.Errorf("roundtrip requires input and output paths"))
		}
		data, err := os.ReadFile(args[1])
		if err != nil {
			return fail(err)
		}
		doc, err := xmltree.Parse(data)
		if err != nil {
			return fail(err)
		}
		result, err := doc.Bytes()
		if err != nil {
			return fail(err)
		}
		if err = fileutil.Write(args[2], result); err != nil {
			return fail(err)
		}
		return 0
	case "rules":
		if len(args) < 2 || args[1] != "validate" || len(args) > 3 {
			return fail(fmt.Errorf("usage: rules validate [FILE]"))
		}
		path := rules.DefaultPath()
		if len(args) == 3 {
			path = args[2]
		}
		if _, err := os.Stat(path); err != nil {
			return fail(err)
		}
		db, err := rules.Load(path)
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(out, "Valid rules database: %d profiles\n", len(db.Profiles))
		return 0
	case "merge":
		fs := flag.NewFlagSet("merge", flag.ContinueOnError)
		fs.SetOutput(errOut)
		base := fs.String("base", "", "common ancestor XML")
		local := fs.String("local", "", "local XML; replaced only on success")
		remote := fs.String("remote", "", "remote XML")
		rulesPath := fs.String("rules", rules.DefaultPath(), "rules XML database")
		userRules := fs.String("user-rules", "", "additional personal rules database")
		output := fs.String("output", "", "result path; defaults to --local")
		interactiveMode := fs.Bool("interactive", false, "open interactive application")
		if err := fs.Parse(args[1:]); err != nil {
			return 2
		}
		if *base == "" || *local == "" || *remote == "" || fs.NArg() != 0 {
			return fail(fmt.Errorf("merge requires --base, --local and --remote"))
		}
		if *output == "" {
			*output = *local
		}
		optArgs := []string{"merge", "--rules", *rulesPath, "--base", *base, "--local", *local, "--remote", *remote, "--output", *output}
		if *userRules != "" {
			optArgs = append(optArgs, "--user-rules", *userRules)
		}
		o, err := interactive.Parse(optArgs, errOut)
		if err != nil {
			return fail(err)
		}
		if *interactiveMode {
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			return interactive.Launch(ctx, o, "", out, errOut)
		}
		b, err := os.ReadFile(*base)
		if err != nil {
			return fail(err)
		}
		l, err := os.ReadFile(*local)
		if err != nil {
			return fail(err)
		}
		r, err := os.ReadFile(*remote)
		if err != nil {
			return fail(err)
		}
		db, err := rules.Load(*rulesPath)
		if err != nil {
			return fail(err)
		}
		if *userRules != "" {
			personal, err := rules.Load(*userRules)
			if err != nil {
				return fail(err)
			}
			db = rules.Overlay(db, personal)
		}
		result, err := merge.Merge(b, l, r, db)
		if err != nil {
			return fail(err)
		}
		if len(result.Learned) > 0 {
			doc, err := xmltree.Parse(l)
			if err != nil {
				return fail(err)
			}
			targetRules := *rulesPath
			if *userRules != "" {
				targetRules = *userRules
			}
			err = rules.Update(targetRules, func(current *rules.Database) error {
				p := current.Profile(doc.Root.Name)
				if p == nil {
					current.Profiles = append(current.Profiles, rules.Profile{Root: doc.Root.Name.Local, Namespace: doc.Root.Name.URI})
					p = &current.Profiles[len(current.Profiles)-1]
				}
				for _, learned := range result.Learned {
					exists := false
					for _, rule := range p.Rules {
						if rule.Path == learned.Path && rule.Selector == learned.Selector {
							exists = true
							break
						}
					}
					if !exists {
						p.Rules = append(p.Rules, learned)
					}
				}
				return nil
			})
			if err != nil {
				return fail(err)
			}
		}
		if len(result.Conflicts) > 0 {
			for _, c := range result.Conflicts {
				fmt.Fprintf(errOut, "Conflict at %s: %s\n", c.Path, c.Reason)
			}
			return 1
		}
		if err = fileutil.Write(*output, result.Data); err != nil {
			return fail(err)
		}
		return 0
	default:
		return fail(fmt.Errorf("unknown command %q", args[0]))
	}
}
