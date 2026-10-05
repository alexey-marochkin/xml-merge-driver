package interactive

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"xmlmerge/internal/rules"
)

const (
	Success   = 0
	Failure   = 2
	Cancelled = 3
)

type Options struct{ Mode, Rules, UserRules, Base, Local, Remote, Output string }

func Parse(args []string, stderr io.Writer) (Options, error) {
	o := Options{Mode: "merge"}
	explicitMode := len(args) > 0 && !strings.HasPrefix(args[0], "-")
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		o.Mode = args[0]
		args = args[1:]
	}
	if o.Mode != "merge" && o.Mode != "compare" && o.Mode != "rules" {
		return o, fmt.Errorf("неизвестный режим %q", o.Mode)
	}
	f := flag.NewFlagSet("xmlmerge-ui", flag.ContinueOnError)
	f.SetOutput(stderr)
	f.StringVar(&o.Rules, "rules", rules.DefaultPath(), "XML-файл правил")
	f.StringVar(&o.UserRules, "user-rules", "", "дополнительная личная база; --rules имеет приоритет")
	f.StringVar(&o.Base, "base", "", "общий предок")
	f.StringVar(&o.Local, "local", "", "локальная версия")
	f.StringVar(&o.Remote, "remote", "", "вторая версия")
	f.StringVar(&o.Output, "output", "", "файл результата")
	if err := f.Parse(args); err != nil {
		return o, err
	}
	if f.NArg() != 0 {
		return o, fmt.Errorf("неизвестные аргументы: %v", f.Args())
	}
	noDocuments := o.Base == "" && o.Local == "" && o.Remote == ""
	if noDocuments && !explicitMode {
		o.Mode = "rules"
	}
	if noDocuments && o.Mode == "rules" {
		if o.Output != "" || o.UserRules != "" {
			return o, fmt.Errorf("отдельный редактор принимает только --rules")
		}
		rulesGiven := false
		f.Visit(func(v *flag.Flag) {
			if v.Name == "rules" {
				rulesGiven = true
			}
		})
		if !rulesGiven {
			exe, err := os.Executable()
			if err != nil {
				return o, err
			}
			o.Rules = filepath.Join(filepath.Dir(exe), "rules.xml")
		}
	} else if o.Base == "" || o.Local == "" || o.Remote == "" {
		return o, fmt.Errorf("нужны --base, --local, --remote и --rules")
	}
	if o.Rules == "" {
		return o, fmt.Errorf("укажите непустой путь --rules")
	}
	if o.Mode != "rules" && o.Output == "" {
		return o, fmt.Errorf("укажите --output; он может совпадать с --local")
	}
	for _, p := range []*string{&o.Rules, &o.UserRules, &o.Base, &o.Local, &o.Remote, &o.Output} {
		if *p != "" {
			abs, err := filepath.Abs(*p)
			if err != nil {
				return o, err
			}
			*p = abs
		}
	}
	for _, settings := range []string{o.Rules, o.UserRules} {
		if settings == "" {
			continue
		}
		for _, data := range []string{o.Base, o.Local, o.Remote, o.Output} {
			if data != "" && SamePath(settings, data) {
				return o, fmt.Errorf("путь настроек совпадает с XML или результатом")
			}
		}
	}
	if o.UserRules != "" && SamePath(o.Rules, o.UserRules) {
		return o, fmt.Errorf("командная и личная базы должны иметь разные пути")
	}
	if o.Output != "" && (SamePath(o.Output, o.Base) || SamePath(o.Output, o.Remote)) {
		return o, fmt.Errorf("результат не должен заменять base или remote")
	}
	return o, nil
}

func SamePath(a, b string) bool {
	if strings.EqualFold(filepath.Clean(a), filepath.Clean(b)) {
		return true
	}
	ai, ae := os.Stat(a)
	bi, be := os.Stat(b)
	return ae == nil && be == nil && os.SameFile(ai, bi)
}

func (o Options) Args() []string {
	a := []string{o.Mode, "--rules", o.Rules}
	if !o.StandaloneRules() {
		a = append(a, "--base", o.Base, "--local", o.Local, "--remote", o.Remote)
	}
	if o.UserRules != "" {
		a = append(a, "--user-rules", o.UserRules)
	}
	if o.Output != "" {
		a = append(a, "--output", o.Output)
	}
	return a
}

func (o Options) StandaloneRules() bool {
	return o.Mode == "rules" && o.Base == "" && o.Local == "" && o.Remote == ""
}
