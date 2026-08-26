package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// saveCtx builds a Ctx with a fake repo root and savedirs configured.
func saveCtx(t *testing.T, ts *httptest.Server, root string) *cli.Ctx {
	return &cli.Ctx{
		Stdout:      &bytes.Buffer{},
		Stderr:      &bytes.Buffer{},
		GlobalFlags: cli.GlobalFlags{Owner: "o", Repo: "r"},
		Cfg:         &config.Config{Savedirs: map[string]string{"issue": ".forge/issues", "pr-conversation": ".forge/prs"}},
		Repo:        &gitctx.Repo{Root: root},
		API:         api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestSaveIssueWritesPrettyJSONAndPrintsPath(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".forge", "issues")
	os.MkdirAll(dir, 0o755)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"number":3,"title":"t"}`)
	}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	if err := (saveCmd{kind: "issue"}).Run([]string{"3"}, ctx); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(dir, "r-3.json")
	if got := ctx.Stdout.(*bytes.Buffer).String(); got != want+"\n" {
		t.Errorf("stdout = %q, want path %s", got, want)
	}
	data, err := os.ReadFile(want)
	if err != nil {
		t.Fatal(err)
	}
	var iss map[string]any
	if err := json.Unmarshal(data, &iss); err != nil {
		t.Fatalf("not valid JSON: %v", err)
	}
}

func TestSaveIssueWithoutSavedirFailsNamingKey(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	ctx.Cfg.Savedirs = map[string]string{}
	err := (saveCmd{kind: "issue"}).Run([]string{"1"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage || !bytes.Contains([]byte(cerr.Msg), []byte("savedir")) {
		t.Fatalf("want ExitUsage naming savedir key, got %v", err)
	}
}

func TestCachePathPrintsResolvedDirs(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	if err := (cachePathCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	if !bytes.Contains([]byte(out), []byte(filepath.Join(root, ".forge/issues"))) ||
		!bytes.Contains([]byte(out), []byte(filepath.Join(root, ".forge/prs"))) {
		t.Errorf("cache path output missing resolved dirs:\n%s", out)
	}
}

func TestCacheFlushRemovesAndReports(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, ".forge", "issues")
	os.MkdirAll(dir, 0o755)
	f := filepath.Join(dir, "r-9.json")
	os.WriteFile(f, []byte("{}"), 0o644)

	ctx := saveCtx(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), root)
	defer func() {}()
	if err := (cacheFlushCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(f); !os.IsNotExist(err) {
		t.Error("cached file not removed")
	}
	if !bytes.Contains(ctx.Stdout.(*bytes.Buffer).Bytes(), []byte("r-9.json")) {
		t.Errorf("flush should report removed paths, got %q", ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestCacheFlushRequiresYesForOutsideRoot(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	os.MkdirAll(outside, 0o755)

	ctx := saveCtx(t, httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})), root)
	ctx.Cfg.Savedirs["issue"] = outside

	err := (cacheFlushCmd{}).Run(nil, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage without --yes, got %v", err)
	}
	err = (cacheFlushCmd{}).Run([]string{"--yes"}, ctx)
	if err != nil {
		t.Fatalf("--yes flush failed: %v", err)
	}
}

func TestSaveCommandsHaveHelpPages(t *testing.T) {
	for _, c := range SaveCommands() {
		pageCmd, ok := c.(interface{ HelpPage() string })
		if !ok {
			t.Errorf("%s does not implement HelpPage", c.Name())
			continue
		}
		if got := pageCmd.HelpPage(); !strings.HasPrefix(got, "use: forge "+c.Name()) {
			t.Errorf("%s help page must start with its synopsis, got %q", c.Name(), got)
		}
	}
}

func TestSaveCmdPagesDifferByKind(t *testing.T) {
	for _, c := range SaveCommands() {
		s, ok := c.(saveCmd)
		if !ok {
			t.Fatalf("unexpected type %T", c)
		}
		if _, ok := any(s).(interface{ HelpPage() string }); !ok {
			t.Fatal("saveCmd lost its HelpPage method")
		}
	}
	prPage := saveCmd{kind: "pr-conversation"}.HelpPage()
	issuePage := saveCmd{kind: "issue"}.HelpPage()
	if prPage == issuePage {
		t.Fatal("both saveCmd kinds return the same help page")
	}
}
