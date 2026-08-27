package cmds

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestCacheCommandsHaveHelpPages(t *testing.T) {
	for _, c := range CacheCommands() {
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

// These three moved here from the deleted save_test.go; they only exercise
// cache path/flush and share the saveCtx harness now defined in pull_test.go.

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
