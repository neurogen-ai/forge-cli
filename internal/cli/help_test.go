package cli

import (
	"bytes"
	"strings"
	"testing"
)

// fakeCmd is a minimal Command for help-system unit tests.
type fakeCmd struct {
	name    string
	summary string
}

func (f *fakeCmd) Name() string                      { return f.name }
func (f *fakeCmd) Summary() string                   { return f.summary }
func (f *fakeCmd) Run(args []string, ctx *Ctx) error { return nil }

func fakeReg() *Registry {
	reg := NewRegistry()
	reg.Register(
		&fakeCmd{name: "a list", summary: "a list summary"},
		&fakeCmd{name: "a get", summary: "a get summary"},
		&fakeCmd{name: "b", summary: "b summary"},
	)
	return reg
}

func TestGroupPrefixGroupsSharedFirstToken(t *testing.T) {
	reg := fakeReg()
	prefix, ok := reg.GroupPrefix("a list")
	if !ok || prefix != "a" {
		t.Fatalf("GroupPrefix(\"a list\") = (%q, %v), want (\"a\", true)", prefix, ok)
	}
	prefix, ok = reg.GroupPrefix("b")
	if ok {
		t.Fatalf("GroupPrefix(\"b\") = (%q, true), want false", prefix)
	}
	// A bare family path also resolves the prefix for `forge <family>`.
	prefix, ok = reg.GroupPrefix("a")
	if !ok || prefix != "a" {
		t.Fatalf("GroupPrefix(\"a\") = (%q, %v), want (\"a\", true)", prefix, ok)
	}
}

func TestHelpTextFallbackFromNameAndSummary(t *testing.T) {
	got := HelpText(&fakeCmd{name: "b", summary: "b summary"})
	want := "use: forge b\n\nb summary"
	if got != want {
		t.Fatalf("HelpText = %q, want %q", got, want)
	}
}

func TestPrintGroupPageForFakePrefix(t *testing.T) {
	var buf bytes.Buffer
	fakeReg().PrintGroupPage(&buf, "a") // no "a" entry in groupSummary
	out := buf.String()
	for _, want := range []string{
		"use: forge a <subcommand>",
		"a list summary",
		"a get summary",
		"run `forge a <subcommand> -h` for details",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("group page missing %q:\n%s", want, out)
		}
	}
	// With no summary entry, the summary line and one blank line are omitted:
	// synopsis, blank, members...; the top-level summaries must not leak.
	lines := strings.Split(out, "\n")
	if lines[1] != "" || !strings.HasPrefix(lines[2], "  ") {
		t.Fatalf("summary-line omission broken; got %q", out)
	}
	if strings.Contains(out, "pull requests") || strings.Contains(out, "issues") ||
		strings.Contains(out, "local savedir maintenance") {
		t.Fatalf("group page leaks unrelated groupSummary entries:\n%s", out)
	}
}

func TestUsageListsFamilyOnceNotPerMember(t *testing.T) {
	var buf bytes.Buffer
	Usage(&buf, fakeReg())
	out := buf.String()
	if n := strings.Count(out, "(forge a -h)"); n != 1 {
		t.Fatalf("family 'a' listed %d times in usage, want 1:\n%s", n, out)
	}
	if strings.Contains(out, "a list summary") || strings.Contains(out, "a get summary") {
		t.Fatalf("usage shows individual family members:\n%s", out)
	}
	if !strings.Contains(out, "b summary") {
		t.Fatalf("standalone command missing from usage:\n%s", out)
	}
}
