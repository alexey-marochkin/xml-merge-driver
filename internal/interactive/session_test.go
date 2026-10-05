package interactive

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"xmlmerge/internal/rules"
	"xmlmerge/internal/xmltree"
)

func fixture(t *testing.T, b, l, r string) Options {
	t.Helper()
	dir := t.TempDir()
	o := Options{Mode: "merge", Rules: filepath.Join(dir, "правила.xml"), Base: filepath.Join(dir, "base.xml"), Local: filepath.Join(dir, "local.xml"), Remote: filepath.Join(dir, "remote.xml"), Output: filepath.Join(dir, "результат.xml")}
	for p, data := range map[string]string{o.Base: b, o.Local: l, o.Remote: r} {
		if e := os.WriteFile(p, []byte(data), 0600); e != nil {
			t.Fatal(e)
		}
	}
	return o
}
func tripleKeyXML() string {
	var b strings.Builder
	b.WriteString("<r>")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&b, `<i a="%d" b="%d" c="%d"/>`, i&1, (i>>1)&1, (i>>2)&1)
	}
	b.WriteString("</r>")
	return b.String()
}

func TestRulesGateAndManualComposite(t *testing.T) {
	xml := tripleKeyXML()
	o := fixture(t, xml, xml, xml)
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	if s.State().Report.Ready {
		t.Fatal("expected a missing three-field key")
	}
	if _, e = s.Compare(); e == nil {
		t.Fatal("comparison bypassed preparation")
	}
	if _, e = s.Tree(""); e == nil {
		t.Fatal("DOM exposed before preparation")
	}
	if e = s.Save("local"); e == nil {
		t.Fatal("saved before preparation")
	}
	rule := rules.Rule{Path: "/{}r/{}i", Mode: "attribute", Order: "insignificant", Fields: []rules.Field{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	p, e := s.Preview(rule)
	if e != nil || !p.Valid {
		t.Fatal(p, e)
	}
	state, e := s.SaveRule(rule, s.State().Revision)
	if e != nil {
		t.Fatal(e)
	}
	if !state.Report.Ready {
		t.Fatal("manual rule did not unlock comparison")
	}
	if _, e = os.Stat(o.Output); !os.IsNotExist(e) {
		t.Fatal("saving rules wrote output")
	}
	state, e = s.Compare()
	if e != nil || !state.CanSave {
		t.Fatal(state, e)
	}
	if e = s.Save("merged"); e != nil {
		t.Fatal(e)
	}
	got, e := os.ReadFile(o.Output)
	if e != nil || string(got) != xml {
		t.Fatal("roundtrip result changed", e)
	}
	if code := s.Close(); code != Success {
		t.Fatal(code)
	}
}

func TestCancelAndStaleOutput(t *testing.T) {
	o := fixture(t, `<r a="0"/>`, `<r a="0"/>`, `<r a="1"/>`)
	o.Output = o.Local
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Compare(); e != nil {
		t.Fatal(e)
	}
	if code := s.Close(); code != Cancelled {
		t.Fatal(code)
	}
	if e = s.Save("merged"); e == nil {
		t.Fatal("saved after cancellation")
	}
	got, _ := os.ReadFile(o.Local)
	if string(got) != `<r a="0"/>` {
		t.Fatal("cancel changed local")
	}
	s, e = New(o)
	if e != nil {
		t.Fatal(e)
	}
	if _, e = s.Compare(); e != nil {
		t.Fatal(e)
	}
	if e = os.WriteFile(o.Output, []byte("external change"), 0600); e != nil {
		t.Fatal(e)
	}
	if e = s.Save("merged"); e == nil {
		t.Fatal("overwrote externally changed output")
	}
}

func TestCloseDoesNotWaitForComparisonLock(t *testing.T) {
	o := fixture(t, `<r/>`, `<r/>`, `<r/>`)
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	s.mu.Lock()
	done := make(chan int, 1)
	go func() { done <- s.Close() }()
	select {
	case code := <-done:
		if code != Cancelled {
			t.Fatal(code)
		}
	case <-time.After(time.Second):
		s.mu.Unlock()
		t.Fatal("close waited for long computation")
	}
	s.mu.Unlock()
}

func TestRootOrderCanBeEdited(t *testing.T) {
	o := fixture(t, `<r><a/><b/></r>`, `<r><a/><b/></r>`, `<r><b/><a/></r>`)
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	r := rules.Rule{Path: "/{}r", Mode: "text", Order: "insignificant"}
	st, e := s.SaveRule(r, s.State().Revision)
	if e != nil {
		t.Fatal(e)
	}
	if !st.Report.Ready {
		t.Fatal("root setting invalidated singleton children")
	}
	if _, e = s.Compare(); e != nil {
		t.Fatal(e)
	}
	if e = s.Save("merged"); e != nil {
		t.Fatal(e)
	}
	got, _ := os.ReadFile(o.Output)
	if string(got) != `<r><a/><b/></r>` {
		t.Fatalf("local order not preserved: %s", got)
	}
}

func TestConcurrentRuleUpdateRejected(t *testing.T) {
	x := tripleKeyXML()
	o := fixture(t, x, x, x)
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	stale := s.State().Revision
	if e = rules.Update(o.Rules, func(db *rules.Database) error { return nil }); e != nil {
		t.Fatal(e)
	}
	r := rules.Rule{Path: "/{}r/{}i", Mode: "attribute", Order: "significant", Fields: []rules.Field{{Name: "a"}, {Name: "b"}, {Name: "c"}}}
	if _, e = s.SaveRule(r, stale); e == nil {
		t.Fatal("stale rule save accepted")
	}
}

func TestTeamRuleHasPriority(t *testing.T) {
	o := fixture(t, `<r><i id="1"/><i id="2"/></r>`, `<r><i id="1"/><i id="2"/></r>`, `<r><i id="1"/><i id="2"/></r>`)
	o.UserRules = filepath.Join(filepath.Dir(o.Rules), "personal.xml")
	for path, field := range map[string]string{o.Rules: "id", o.UserRules: "missing"} {
		if e := rules.Update(path, func(db *rules.Database) error {
			db.Put(xmltree.Name{Local: "r"}, rules.Rule{Path: "/{}r/{}i", Mode: "attribute", Order: "significant", Fields: []rules.Field{{Name: field}}})
			return nil
		}); e != nil {
			t.Fatal(e)
		}
	}
	s, e := New(o)
	if e != nil {
		t.Fatal(e)
	}
	if !s.State().Report.Ready {
		t.Fatal("personal rule overrode team rule")
	}
}

func TestPathsAndArguments(t *testing.T) {
	o, e := Parse([]string{"merge", "--rules", `папка с пробелами\правила.xml`, "--base", "b.xml", "--local", "l.xml", "--remote", "r.xml", "--output", "l.xml"}, io.Discard)
	if e != nil {
		t.Fatal(e)
	}
	if !filepath.IsAbs(o.Rules) {
		t.Fatal("paths not resolved")
	}
	o2, e := Parse(o.Args(), io.Discard)
	if e != nil || o2 != o {
		t.Fatal(o2, e)
	}
	if _, e = Parse([]string{"merge", "--rules", "l.xml", "--base", "b.xml", "--local", "l.xml", "--remote", "r.xml", "--output", "out.xml"}, io.Discard); e == nil {
		t.Fatal("rules/data alias allowed")
	}
}

func TestMain(m *testing.M) {
	if os.Getenv("XMLMERGE_LAUNCH_TEST_HELPER") == "1" {
		data, _ := json.Marshal(os.Args[1:])
		_ = os.WriteFile(os.Getenv("XMLMERGE_LAUNCH_TEST_ARGS"), data, 0600)
		if os.Getenv("XMLMERGE_LAUNCH_TEST_WAIT") == "1" {
			time.Sleep(30 * time.Second)
		}
		code, _ := strconv.Atoi(os.Getenv("XMLMERGE_LAUNCH_TEST_CODE"))
		os.Exit(code)
	}
	os.Exit(m.Run())
}

func TestLaunchForwardsArgumentsAndExit(t *testing.T) {
	self, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	o := fixture(t, `<r/>`, `<r/>`, `<r/>`)
	t.Setenv("XMLMERGE_LAUNCH_TEST_HELPER", "1")
	argsFile := filepath.Join(t.TempDir(), "args.json")
	t.Setenv("XMLMERGE_LAUNCH_TEST_ARGS", argsFile)
	for _, code := range []int{Success, Cancelled, Failure} {
		t.Setenv("XMLMERGE_LAUNCH_TEST_CODE", strconv.Itoa(code))
		var log bytes.Buffer
		if got := Launch(context.Background(), o, self, &log, &log); got != code {
			t.Fatalf("got %d expected %d: %s", got, code, log.String())
		}
		data, _ := os.ReadFile(argsFile)
		var args []string
		if e := json.Unmarshal(data, &args); e != nil {
			t.Fatal(e)
		}
		if !reflect.DeepEqual(args, o.Args()) {
			t.Fatal("changed CLI arguments", args)
		}
	}
	t.Setenv("XMLMERGE_LAUNCH_TEST_WAIT", "1")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if code := Launch(ctx, o, self, io.Discard, io.Discard); code != Cancelled {
		t.Fatal(code)
	}
}
