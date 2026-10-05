// Package gitdriver implements the supplier-selection policy from mergexml.py.
// It is separate from the generic XML merge engine and its identification rules.
package gitdriver

import (
	"bytes"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"

	"xmlmerge/internal/xmltree"
)

type Decision int

const (
	Merge Decision = iota
	KeepLocal
	TakeRemote
)

// Report explains a policy decision without including XML contents.
type Report struct {
	Reason   string
	Steps    []string
	detailed bool
}

func (r *Report) trace(format string, args ...any) {
	r.Steps = append(r.Steps, fmt.Sprintf(format, args...))
}

func SelectWithReport(policy *Policy, path, remoteLabel string, local, remote []byte, detailed bool) (Decision, Report, error) {
	report := Report{detailed: detailed}
	decision, err := selectWithGitReport(policy, path, remoteLabel, local, remote, git, &report)
	return decision, report, err
}

func (p *Policy) EligibleFile(path string) bool {
	name := filepath.Base(strings.ReplaceAll(path, "\\", "/"))
	for _, candidate := range p.Files {
		if name == candidate {
			return true
		}
	}
	return false
}

// Select avoids Git calls for ordinary substantive changes.
// Failed label resolution means no evidence of supplier ancestry, not success.
func Select(policy *Policy, path, remoteLabel string, local, remote []byte) (Decision, error) {
	return selectWithGit(policy, path, remoteLabel, local, remote, git)
}

type gitCall func(...string) (string, int, error)

func selectWithGit(policy *Policy, path, remoteLabel string, local, remote []byte, run gitCall) (Decision, error) {
	return selectWithGitReport(policy, path, remoteLabel, local, remote, run, &Report{})
}

func selectWithGitReport(policy *Policy, path, remoteLabel string, local, remote []byte, run gitCall, report *Report) (Decision, error) {
	eligibility := ""
	choose := func(decision Decision, reason string) (Decision, error) {
		report.Reason = reason
		if eligibility != "" {
			report.Reason += "; " + eligibility
		}
		return decision, nil
	}
	report.trace("remote-метка: %q", remoteLabel)
	basename := filepath.Base(strings.ReplaceAll(path, "\\", "/"))
	for _, name := range policy.KeepLocalFiles {
		if basename == name {
			report.trace("Явное правило сохранения local; проверка поставщика не требуется")
			return choose(KeepLocal, "файл из списка keep-local-files")
		}
	}
	if bytes.Equal(local, remote) {
		report.trace("Проверка Git не требуется: local и remote побайтно совпадают")
		return choose(KeepLocal, "local и remote совпадают")
	}
	var changeRule string
	if policy.EligibleFile(path) {
		eligibility = "файл из защищённого списка"
	} else if onlyChanges(local, remote, policy.Changes, &changeRule) {
		eligibility = "только допустимые изменения атрибутов: " + changeRule
	} else {
		report.trace("Проверка Git не требуется: нет основания выбирать файл целиком")
		return choose(Merge, "политика выбора файла целиком неприменима")
	}
	report.trace("Основание политики: %s", eligibility)
	report.trace("Ветки поставщика в настройках: %q", policy.SupplierBranches())
	head, code, err := run("symbolic-ref", "--quiet", "--short", "HEAD")
	if err != nil || code > 1 {
		return Merge, fmt.Errorf("cannot determine current Git branch: %s (%v)", head, err)
	}
	if code == 0 {
		report.trace("Текущая ветка: %q", head)
	} else {
		report.trace("HEAD не указывает на ветку (detached HEAD)")
	}
	supplierRefs := map[string]bool{}
	for _, branch := range policy.SupplierBranches() {
		if code == 0 && head == branch {
			report.trace("Поставщик local: текущая ветка %q входит в список; проверка истории не нужна", branch)
			return choose(KeepLocal, fmt.Sprintf("поставщик local, ветка %q", branch))
		}
		supplierRefs["refs/heads/"+branch] = true
	}
	for _, part := range policy.RemotePaths {
		if strings.Contains(path, part) {
			report.trace("Сработало старое исключение пути %q; сторона поставщика не проверяется", part)
			return choose(TakeRemote, fmt.Sprintf("исключение пути %q", part))
		}
	}
	if remoteLabel == "" || strings.Contains(remoteLabel, "Temporary merge branch") {
		return choose(Merge, "нет пригодной remote-метки для определения поставщика")
	}
	label, _, _ := strings.Cut(remoteLabel, ":")
	commit, code, err := run("rev-parse", "--verify", "--end-of-options", label+"^{commit}")
	if err != nil {
		return Merge, err
	}
	if code != 0 {
		return choose(Merge, "remote-ревизия не разрешается в коммит")
	}
	report.trace("remote-коммит: %s", commit)
	// An explicit working branch keeps its role even when its tip is shared
	// with a supplier branch. Only unnamed revisions use history as a fallback.
	ref, code, err := run("rev-parse", "--symbolic-full-name", "--verify", "--end-of-options", label)
	if err != nil {
		return Merge, err
	}
	if code != 0 {
		return choose(Merge, "не удалось однозначно разрешить remote-ссылку")
	}
	report.trace("remote-ссылка: %q", ref)
	if strings.HasPrefix(ref, "refs/heads/") || strings.HasPrefix(ref, "refs/remotes/") {
		if supplierRefs[ref] {
			report.trace("Поставщик remote: ссылка %q входит в список; проверка истории не нужна", ref)
			return choose(TakeRemote, fmt.Sprintf("поставщик remote, ветка %q", ref))
		}
		report.trace("Явно указана ветка вне списка поставщиков; общая история не меняет её роль")
		return choose(Merge, fmt.Sprintf("remote-ветка %q не является поставщиком", ref))
	}
	// Enumerate existing exact refs first: a configured supplier can be absent
	// in this repository, and Git's ref patterns can also return child refs.
	args := []string{"for-each-ref", "--format=%(refname)"}
	for _, branch := range policy.SupplierBranches() {
		args = append(args, "refs/heads/"+branch)
	}
	branches, code, err := run(args...)
	if err != nil || code != 0 {
		return Merge, fmt.Errorf("cannot list supplier branches: %s (%v)", branches, err)
	}
	args = []string{"rev-list", "--first-parent"}
	for _, branch := range strings.Split(branches, "\n") {
		if supplierRefs[branch] {
			args = append(args, branch)
		}
	}
	if len(args) == 2 {
		return choose(Merge, "в репозитории нет настроенных веток поставщика")
	}
	report.trace("Проверка цепочек первых родителей: %q", args[2:])
	existingRefs := append([]string(nil), args[2:]...)
	// Ancestors of the candidate cannot affect membership of the candidate
	// itself. Excluding them avoids walking the entire older supplier history.
	args = append(args, "--not", commit+"^@", "--")
	history, code, err := run(args...)
	if err != nil || code != 0 {
		return Merge, fmt.Errorf("cannot check supplier first-parent history: %s (%v)", history, err)
	}
	for _, candidate := range strings.Split(history, "\n") {
		if candidate == commit {
			source := "веток поставщика"
			if len(existingRefs) == 1 {
				source = fmt.Sprintf("%q", existingRefs[0])
			} else if report.detailed {
				// The combined check establishes the decision. Extra checks only
				// attribute it to a named branch for detailed diagnostics.
				for _, branch := range existingRefs {
					branchHistory, status, checkErr := run("rev-list", "--first-parent", branch, "--not", commit+"^@", "--")
					if checkErr != nil || status != 0 {
						report.trace("Не удалось уточнить имя ветки %q; общий результат проверки остаётся действительным", branch)
						break
					}
					found := false
					for _, id := range strings.Split(branchHistory, "\n") {
						if id == commit {
							found = true
							break
						}
					}
					if found {
						source = fmt.Sprintf("%q", branch)
						break
					}
				}
			}
			report.trace("Коммит найден в основной цепочке %s; поставщик remote", source)
			return choose(TakeRemote, "коммит поставщика в основной цепочке "+source)
		}
	}
	report.trace("Коммит не найден в цепочках первых родителей")
	return choose(Merge, "remote-коммит вне основных цепочек поставщика")
}

func git(args ...string) (string, int, error) {
	cmd := exec.Command("git", args...)
	data, err := cmd.Output()
	if err == nil {
		return strings.TrimSpace(string(data)), 0, nil
	}
	if e, ok := err.(*exec.ExitError); ok {
		return strings.TrimSpace(string(e.Stderr)), e.ExitCode(), nil
	}
	return "", -1, err
}

// OnlyChanges requires exactly one update of each attribute in an allowed
// combination, with every other physical detail intact. Attribute names are
// unqualified. No fuzzy matching, sorting or text/CDATA normalization.
func OnlyChanges(local, remote []byte, combinations []Change) bool {
	return onlyChanges(local, remote, combinations, nil)
}

func onlyChanges(local, remote []byte, combinations []Change, matchedRule *string) bool {
	if len(combinations) == 0 {
		return false
	}
	allowed := map[string]bool{}
	for _, c := range combinations {
		for _, a := range c.Attributes {
			allowed[a.Name] = true
		}
	}
	l, err := xmltree.Parse(local)
	if err != nil {
		return false
	}
	r, err := xmltree.Parse(remote)
	if err != nil || l.Format.Encoding != r.Format.Encoding || !bytes.Equal(l.Format.BOM, r.Format.BOM) {
		return false
	}
	changed := map[string]int{}
	var compare func([]xmltree.Part, []xmltree.Part) bool
	compare = func(a, b []xmltree.Part) bool {
		if len(a) != len(b) {
			return false
		}
		for i, left := range a {
			right := b[i]
			if left.Kind != right.Kind || left.Raw != right.Raw {
				return false
			}
			if left.Element == nil || right.Element == nil {
				if left.Element != right.Element {
					return false
				}
				continue
			}
			x, y := left.Element, right.Element
			if x.Name != y.Name || x.LexicalName != y.LexicalName || len(x.Attributes) != len(y.Attributes) {
				return false
			}
			for j, attr := range x.Attributes {
				other := y.Attributes[j]
				if attr.Name != other.Name || attr.LexicalName != other.LexicalName {
					return false
				}
				if attr.Value == other.Value {
					continue
				}
				if !allowed[attr.LexicalName] {
					return false
				}
				changed[attr.LexicalName]++
				if changed[attr.LexicalName] > 1 {
					return false
				}
				// ReplaceAttribute changes only the differing value; final byte equality
				// catches quote/order/layout differences, including inside CDATA.
				x.ReplaceAttribute(attr.Name, &other)
			}
			if !compare(x.Parts, y.Parts) {
				return false
			}
		}
		return true
	}
	if !compare(l.Parts, r.Parts) || len(changed) == 0 {
		return false
	}
	matches := false
	var matchedAttributes []Attribute
	for _, c := range combinations {
		if len(c.Attributes) != len(changed) {
			continue
		}
		matches = true
		for _, a := range c.Attributes {
			if changed[a.Name] != 1 {
				matches = false
				break
			}
		}
		if matches {
			matchedAttributes = c.Attributes
			break
		}
	}
	if !matches {
		return false
	}
	patched, err := l.Bytes()
	if err != nil || !bytes.Equal(patched, remote) {
		return false
	}
	if matchedRule != nil {
		names := make([]string, len(matchedAttributes))
		for i, attr := range matchedAttributes {
			names[i] = attr.Name
		}
		*matchedRule = strings.Join(names, " + ")
	}
	return true
}
