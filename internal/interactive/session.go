package interactive

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"

	"xmlmerge/internal/fileutil"
	"xmlmerge/internal/merge"
	"xmlmerge/internal/prepare"
	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

type State struct {
	Database  *rules.Database `json:",omitempty"`
	Options   Options
	Root      string
	Report    *prepare.Report
	Revision  string
	Phase     string
	Conflicts []merge.Conflict
	CanSave   bool
	CanUndo   bool
	Saved     bool
	Warnings  []string
	Changes   int
	Decisions int
}

type Session struct {
	mu            sync.Mutex
	commit        sync.Mutex
	Options       Options
	docs          []*xmltree.Document
	input         [3][]byte
	db            *rules.Database
	report        *prepare.Report
	revision      string
	phase         string
	conflicts     []merge.Conflict
	moveConflicts []merge.Conflict
	result        []byte
	saved         bool
	closed        atomic.Bool
	warnings      []string
	outputBefore  []byte
	outputExisted bool
	nodes         map[string]*reviewNode
	nodeOrder     []string
	decisions     map[string]NodeDecision
	undo          []map[string]NodeDecision
	center        map[string]*xmltree.Node
	initialCenter map[string]*xmltree.Node
}

func New(o Options) (*Session, error) {
	s := &Session{Options: o, phase: "rules"}
	if o.StandaloneRules() {
		if err := rules.Ensure(o.Rules); err != nil {
			return nil, fmt.Errorf("файл правил %s: %w", o.Rules, err)
		}
		if err := s.refresh(false); err != nil {
			return nil, err
		}
		return s, nil
	}
	for i, p := range []string{o.Base, o.Local, o.Remote} {
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, err
		}
		doc, err := xmltree.Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", p, err)
		}
		s.input[i] = data
		s.docs = append(s.docs, doc)
	}
	if o.Output != "" {
		data, err := os.ReadFile(o.Output)
		if err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		s.outputBefore, s.outputExisted = data, err == nil
	}
	if err := s.refresh(true); err != nil {
		return nil, err
	}
	return s, nil
}

func revision(paths ...string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		if p == "" {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil && !os.IsNotExist(err) {
			return "", err
		}
		fmt.Fprintf(h, "%s:%t:%d:", p, err == nil, len(data))
		h.Write(data)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func (s *Session) refresh(auto bool) error {
	before, err := revision(s.Options.Rules, s.Options.UserRules)
	if err != nil {
		return err
	}
	team, err := rules.Load(s.Options.Rules)
	if err != nil {
		return err
	}
	personal := rules.New()
	if s.Options.UserRules != "" {
		personal, err = rules.Load(s.Options.UserRules)
		if err != nil {
			return err
		}
	}
	db := rules.Overlay(team, personal)
	if s.Options.StandaloneRules() {
		after, err := revision(s.Options.Rules)
		if err != nil {
			return err
		}
		if before != after {
			return fmt.Errorf("настройки изменились во время чтения; обновите их")
		}
		s.db, s.revision, s.phase = team, before, "catalog"
		return nil
	}
	r, err := prepare.Analyze(s.docs, db, auto)
	if err != nil {
		return err
	}
	after, err := revision(s.Options.Rules, s.Options.UserRules)
	if err != nil {
		return err
	}
	if before != after {
		return fmt.Errorf("настройки изменились во время чтения; обновите их")
	}
	if len(r.Learned) > 0 {
		target := s.Options.Rules
		if s.Options.UserRules != "" {
			target = s.Options.UserRules
		}
		err = rules.Update(target, func(current *rules.Database) error {
			actual, err := revision(s.Options.Rules, s.Options.UserRules)
			if err != nil {
				return err
			}
			if actual != before {
				return fmt.Errorf("настройки изменены другим процессом; повторите подготовку")
			}
			p := current.Profile(s.docs[0].Root.Name)
			for _, learned := range r.Learned {
				exists := false
				if p != nil {
					for _, existing := range p.Rules {
						if existing.Path == learned.Path && existing.Selector == learned.Selector {
							exists = true
							break
						}
					}
				}
				if !exists {
					current.Put(s.docs[0].Root.Name, learned)
				}
			}
			return nil
		})
		if err != nil {
			return fmt.Errorf("сохранение автоматически найденных правил: %w", err)
		}
		return s.refresh(false)
	}
	s.db, s.report = db, r
	s.revision = before
	s.phase = "rules"
	s.result = nil
	s.conflicts = nil
	s.moveConflicts = nil
	s.initialCenter = nil
	s.nodes = nil
	s.decisions = nil
	s.undo = nil
	return err
}

func (s *Session) state() State {
	if s.Options.StandaloneRules() {
		return State{Options: s.Options, Database: rules.Clone(s.db), Revision: s.revision, Phase: "catalog"}
	}
	return State{Options: s.Options, Root: s.docs[0].Root.Name.String(), Report: s.report, Revision: s.revision, Phase: s.phase, Conflicts: s.conflicts, CanSave: len(s.result) > 0 && !s.closed.Load(), CanUndo: len(s.undo) > 0 && !s.closed.Load(), Saved: s.saved, Warnings: s.warnings, Changes: s.changeCount(), Decisions: len(s.decisions)}
}
func (s *Session) State() State { s.mu.Lock(); defer s.mu.Unlock(); return s.state() }
func (s *Session) Reload() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return s.state(), fmt.Errorf("операция завершена")
	}
	err := s.refresh(true)
	return s.state(), err
}
func (s *Session) group(path string) *prepare.Group {
	if s.report == nil {
		return nil
	}
	for _, g := range s.report.Groups {
		if g.Path == path {
			return g
		}
	}
	return nil
}

func (s *Session) Preview(rule rules.Rule) (prepare.Preview, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(rule.Path)
	if g == nil {
		return prepare.Preview{}, fmt.Errorf("узел не найден")
	}
	g, rule, err := s.ruleScope(g, rule)
	if err != nil {
		return prepare.Preview{}, err
	}
	db := rules.Clone(s.db)
	db.Put(s.docs[0].Root.Name, rule)
	if err := db.Validate(); err != nil {
		return prepare.Preview{}, err
	}
	return prepare.PreviewRule(g, rule), nil
}

func (s *Session) SaveRule(rule rules.Rule, expected string) (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return s.state(), fmt.Errorf("операция завершена")
	}
	g := s.group(rule.Path)
	if g == nil {
		return s.state(), fmt.Errorf("узел не найден")
	}
	g, rule, scopeErr := s.ruleScope(g, rule)
	if scopeErr != nil {
		return s.state(), scopeErr
	}
	if p := prepare.PreviewRule(g, rule); !p.Valid {
		return s.state(), fmt.Errorf("%s", p.Error)
	}
	rule.Origin = "manual"
	err := rules.Update(s.Options.Rules, func(current *rules.Database) error {
		actual, err := revision(s.Options.Rules, s.Options.UserRules)
		if err != nil {
			return err
		}
		if expected != s.revision || actual != expected {
			return fmt.Errorf("настройки изменены другим процессом; обновите их перед сохранением")
		}
		current.Put(s.docs[0].Root.Name, rule)
		return nil
	})
	if err == nil {
		err = s.refresh(true)
	}
	return s.state(), err
}

func (s *Session) ruleScope(g *prepare.Group, rule rules.Rule) (*prepare.Group, rules.Rule, error) {
	if rule.Selector == "" {
		return g, rule, nil
	}
	if rule.Selector != g.Selector {
		return nil, rule, fmt.Errorf("имя правила не соответствует выбранному элементу")
	}
	all := *g
	all.Sets = nil
	for _, other := range s.report.Groups {
		if other.Selector == g.Selector {
			all.Sets = append(all.Sets, other.Sets...)
		}
	}
	rule.Path = ""
	return &all, rule, nil
}

func (s *Session) Compare() (State, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() {
		return s.state(), fmt.Errorf("операция завершена")
	}
	if s.report == nil || !s.report.Ready {
		return s.state(), fmt.Errorf("сначала настройте все недостающие правила")
	}
	if s.Options.Mode == "rules" {
		return s.state(), fmt.Errorf("открыт режим редактирования правил")
	}
	if s.nodes == nil {
		s.buildReview()
	}
	return s.recompare()

}

// Save is an explicit user action; node decisions only change the in-memory
// candidate. Choosing an entire source remains an explicit alternative.
func (s *Session) Save(choice string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed.Load() || s.phase != "compared" || !s.report.Ready {
		return fmt.Errorf("сначала выполните сравнение")
	}
	data := s.result
	if choice == "local" || choice == "remote" {
		side := 1
		if choice == "remote" {
			side = 2
		}
		doc, err := xmltree.Parse(s.input[1])
		if err != nil {
			return err
		}
		for i, p := range doc.Parts {
			if p.Element != nil {
				doc.Parts[i].Element = s.docs[side].Root
				doc.Root = s.docs[side].Root
			}
		}
		data, err = doc.Bytes()
		if err != nil {
			return err
		}
	} else if choice != "merged" {
		return fmt.Errorf("неизвестный вариант результата")
	}
	if len(data) == 0 {
		return fmt.Errorf("неразрешенные конфликты")
	}
	if _, err := xmltree.Parse(data); err != nil {
		return err
	}
	s.commit.Lock()
	defer s.commit.Unlock()
	if s.closed.Load() {
		return fmt.Errorf("операция отменена")
	}
	if err := os.MkdirAll(filepath.Dir(s.Options.Output), 0700); err != nil {
		return err
	}
	// Serialize competing saves from our own UI processes and reject stale output.
	lock, err := os.OpenFile(s.Options.Output+".xmlmerge-lock", os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("результат занят другой операцией: %w", err)
	}
	lock.Close()
	defer os.Remove(s.Options.Output + ".xmlmerge-lock")
	current, err := os.ReadFile(s.Options.Output)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	if (err == nil) != s.outputExisted || !bytes.Equal(current, s.outputBefore) {
		return fmt.Errorf("файл результата изменился после запуска; сохранение отменено")
	}
	if err = fileutil.Write(s.Options.Output, data); err != nil {
		return err
	}
	s.saved = true
	s.closed.Store(true)
	return nil
}

func (s *Session) Close() int {
	// Cancellation never waits for a long comparison; only a final atomic save
	// may finish first. The process can then terminate and release its listener.
	s.commit.Lock()
	defer s.commit.Unlock()
	s.closed.Store(true)
	if s.saved || s.Options.StandaloneRules() {
		return Success
	}
	return Cancelled
}

type Sample struct {
	Side       string
	Group      int
	Index      int
	Attributes map[string]string
	Elements   map[string]string
	Text       string
}

func (s *Session) Samples(path string) ([]Sample, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	g := s.group(path)
	if g == nil {
		return nil, fmt.Errorf("узел не найден")
	}
	var out []Sample
	for gi, set := range g.Sets {
		for i, n := range set.Nodes {
			if i >= 4 {
				break
			}
			row := Sample{Side: set.Side, Group: gi + 1, Index: i + 1, Attributes: map[string]string{}, Elements: map[string]string{}, Text: short(n.DirectText())}
			for _, a := range n.Attributes {
				row.Attributes[a.Name.String()] = short(a.Value)
			}
			for _, c := range n.Children() {
				if len(c.Children()) == 0 {
					row.Elements[c.Name.String()] = short(c.DirectText())
				}
			}
			out = append(out, row)
			if len(out) >= 24 {
				return out, nil
			}
		}
	}
	return out, nil
}
func short(s string) string {
	r := []rune(s)
	if len(r) > 240 {
		return string(r[:240]) + "…"
	}
	return s
}

// MarshaledState provides a detached snapshot for HTTP encoding outside the lock.
func (s *Session) MarshaledState() ([]byte, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return json.Marshal(s.state())
}
