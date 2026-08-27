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
	"sync"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
)

// saveCtx builds a Ctx with a fake repo root and savedirs configured.
// (Inherited from the deleted save_test.go harness; shared by pull and
// cache tests.)
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

func TestPullPRWritesCacheAndReceipt(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/42/comments":
			fmt.Fprint(w, `[{"id":1,"user":{"login":"a"},"body":"c1"},{"id":2,"user":{"login":"a2"},"body":"c2"}]`)
		case "/api/v1/repos/o/r/pulls/42/reviews":
			fmt.Fprint(w, `[{"id":10,"state":"APPROVED","user":{"login":"b"},"commit_id":"abc","stale":true},{"id":11,"state":"CHANGES_REQUESTED","user":{"login":"c"}}]`)
		case "/api/v1/repos/o/r/pulls/42/reviews/10/comments":
			fmt.Fprint(w, `[{"id":20,"path":"x.go","resolved":true},{"id":21,"path":"y.go"}]`)
		case "/api/v1/repos/o/r/pulls/42/reviews/11/comments":
			fmt.Fprint(w, `[{"id":22,"path":"z.go"}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	if err := (pullCmd{kind: "pr"}).Run([]string{"42"}, ctx); err != nil {
		t.Fatal(err)
	}

	var receipt PullReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatalf("stdout not a PullReceipt: %v", err)
	}
	if receipt.Items != 5 { // 2 issue comments + 3 review comments
		t.Errorf("receipt.Items = %d, want 5", receipt.Items)
	}
	if receipt.Reviews == nil || *receipt.Reviews != 2 {
		t.Errorf("receipt.Reviews = %v, want 2", receipt.Reviews)
	}
	if receipt.Unresolved == nil || *receipt.Unresolved != 2 { // ids 21 and 22
		t.Errorf("receipt.Unresolved = %v, want 2", receipt.Unresolved)
	}
	wantPath := filepath.Join(root, ".forge", "prs", "r-42.json")
	if receipt.Path != wantPath {
		t.Errorf("receipt.Path = %q, want %q", receipt.Path, wantPath)
	}
	data, err := os.ReadFile(wantPath)
	if err != nil {
		t.Fatalf("cache file missing: %v", err)
	}
	var cache PRCache
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("not decodable as PRCache: %v", err)
	}
	if len(cache.Comments) != 2 || len(cache.Reviews) != 2 {
		t.Fatalf("cache shape wrong: %+v", cache)
	}
	r0 := cache.Reviews[0]
	if r0.ID != 10 || r0.State != "APPROVED" || r0.CommitID != "abc" || !r0.Stale {
		t.Errorf("review 10 fields wrong: %+v", r0)
	}
	if len(r0.Comments) != 2 {
		t.Fatalf("review 10 should nest 2 comments: %+v", r0.Comments)
	}
	if got := r0.Comments[0].ID; got != 20 {
		t.Errorf("first nested comment id = %d, want server order (20)", got)
	}
	if got := r0.Comments[0].Path; got != "x.go" {
		t.Errorf("nested comment path = %q, want x.go", got)
	}
	if !r0.Comments[0].IsResolved() {
		t.Error("review-comment 20 should decode with resolved=true")
	}
	if cache.Reviews[1].Comments[0].IsResolved() {
		t.Error("review-comment 22 must count as unresolved (no resolver field)")
	}
}

func TestPullPRSecondInvocationReplacesWithoutGrowth(t *testing.T) {
	root := t.TempDir()
	var mu sync.Mutex
	issueComments := `[{"id":1,"body":"before"}]`
	fetchIssueComments := func() string {
		mu.Lock()
		defer mu.Unlock()
		return issueComments
	}
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/42/comments":
			fmt.Fprint(w, fetchIssueComments())
		case "/api/v1/repos/o/r/pulls/42/reviews":
			fmt.Fprint(w, `[{"id":10,"state":"APPROVED","user":{"login":"b"}}]`)
		case "/api/v1/repos/o/r/pulls/42/reviews/10/comments":
			fmt.Fprint(w, `[]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	cmd := pullCmd{kind: "pr"}
	for i := 1; i <= 2; i++ {
		if i == 2 {
			mu.Lock()
			issueComments = `[{"id":1,"body":"after"}]`
			mu.Unlock()
		}
		ctx.Stdout.(*bytes.Buffer).Reset()
		if err := cmd.Run([]string{"42"}, ctx); err != nil {
			t.Fatal(err)
		}
	}

	path := filepath.Join(root, ".forge", "prs")
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "r-42.json" {
		t.Fatalf("savedir holds %d entries (%v), want exactly r-42.json", len(entries), entries)
	}
	data, err := os.ReadFile(filepath.Join(path, "r-42.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cache PRCache
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatal(err)
	}
	if len(cache.Comments) != 1 || cache.Comments[0].Body != "after" {
		t.Errorf("newer snapshot did not replace older: %+v", cache.Comments)
	}
}

func TestPullIssueWrapsPayloadAndCountsItems(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/5":
			fmt.Fprint(w, `{"number":5,"title":"t"}`)
		case "/api/v1/repos/o/r/issues/5/comments":
			fmt.Fprint(w, `[{"id":9,"body":"x"},{"id":10,"body":"y"}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	if err := (pullCmd{kind: "issue"}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	var receipt PullReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Items != 3 { // len(comments)+1
		t.Errorf("receipt.Items = %d, want 3", receipt.Items)
	}
	if receipt.Reviews != nil || receipt.Unresolved != nil {
		t.Errorf("issue receipt must omit PR-only fields: %+v", receipt)
	}
	data, err := os.ReadFile(filepath.Join(root, ".forge", "issues", "r-5.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cache IssueCache
	if err := json.Unmarshal(data, &cache); err != nil {
		t.Fatalf("not decodable as IssueCache: %v", err)
	}
	if cache.Issue == nil || cache.Issue.Title != "t" || len(cache.Comments) != 2 {
		t.Errorf("issue cache nesting wrong: %+v", cache)
	}
}

func TestPullMissingSavedirKeyNamesKeyAndExitsUsage(t *testing.T) {
	for kind, key := range map[string]string{"pr": "pr-conversation", "issue": "issue"} {
		root := t.TempDir()
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
		defer ts.Close()

		ctx := saveCtx(t, ts, root)
		ctx.Cfg.Savedirs = map[string]string{}
		err := (pullCmd{kind: kind}).Run([]string{"1"}, ctx)
		cerr, ok := err.(*cli.Error)
		if !ok || cerr.Code != cli.ExitUsage {
			t.Fatalf("%s: want ExitUsage, got %v", kind, err)
		}
		if !strings.Contains(cerr.Msg, "savedir") || !strings.Contains(cerr.Msg, key) {
			t.Errorf("%s: error must name the savedir key %q, msg %q", kind, key, cerr.Msg)
		}
	}
}

func TestPullReceiptIgnoresFormatFlags(t *testing.T) {
	root := t.TempDir()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/issues/7":
			fmt.Fprint(w, `{"number":7}`)
		case "/api/v1/repos/o/r/issues/7/comments":
			fmt.Fprint(w, `[{"id":1}]`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := saveCtx(t, ts, root)
	cmd := pullCmd{kind: "issue"}
	if err := cmd.Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	first := append([]byte(nil), ctx.Stdout.(*bytes.Buffer).Bytes()...)

	ctx.Format = cli.FormatTable
	ctx.Stdout.(*bytes.Buffer).Reset()
	if err := cmd.Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	second := ctx.Stdout.(*bytes.Buffer).Bytes()
	if !bytes.Equal(first, second) {
		t.Errorf("receipt must ignore --table:\nfirst  %s\nsecond %s", first, second)
	}
	var receipt PullReceipt
	if err := json.Unmarshal(second, &receipt); err != nil {
		t.Fatalf("FormatTable run did not print JSON: %v", err)
	}
}

func TestPullCommandsHaveHelpPages(t *testing.T) {
	pages := make(map[string]string)
	for _, c := range PullCommands() {
		pageCmd, ok := c.(interface{ HelpPage() string })
		if !ok {
			t.Errorf("%s does not implement HelpPage", c.Name())
			continue
		}
		got := pageCmd.HelpPage()
		if !strings.HasPrefix(got, "use: forge "+c.Name()) {
			t.Errorf("%s help page must start with its synopsis, got %q", c.Name(), got)
		}
		pages[c.Name()] = got
	}
	if pages["pr pull"] == pages["issue pull"] {
		t.Error("both pull kinds return the same help page")
	}
}
