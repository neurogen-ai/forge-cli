package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestIssueCreateResolvesLabelNames(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/labels":
			fmt.Fprint(w, `[{"id":1,"name":"bug"},{"id":2,"name":"docs"}]`)
		case "/api/v1/repos/o/r/issues":
			json.NewDecoder(r.Body).Decode(&gotBody)
			fmt.Fprint(w, `{"number":12,"title":"t"}`)
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (issueCreateCmd{}).Run([]string{"--title", "t", "--label", "bug", "--label", "docs"}, ctx); err != nil {
		t.Fatal(err)
	}
	labels, _ := gotBody["labels"].([]any)
	if len(labels) != 2 || labels[0] != float64(1) || labels[1] != float64(2) {
		t.Errorf("labels = %v, want ids [1 2]", gotBody["labels"])
	}
}

func TestIssueCreateUnknownLabelFailsWithoutPOST(t *testing.T) {
	posted := false
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/repos/o/r/labels":
			fmt.Fprint(w, `[{"id":1,"name":"bug"}]`)
		case "/api/v1/repos/o/r/issues":
			posted = true
			fmt.Fprint(w, `{}`)
		}
	}))
	defer ts.Close()

	err := (issueCreateCmd{}).Run([]string{"--title", "t", "--label", "nope"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
	if !bytes.Contains([]byte(cerr.Msg), []byte("nope")) {
		t.Errorf("error should name the unknown label, got %q", cerr.Msg)
	}
	if posted {
		t.Error("issue must not be created when a label name is unknown")
	}
}

func TestIssueListSendsTypeIssues(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		fmt.Fprint(w, `[]`)
	}))
	defer ts.Close()

	if err := (issueListCmd{}).Run([]string{}, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "page=1&type=issues" {
		t.Errorf("query = %q", gotQuery)
	}
}

func TestIssueListPipedStillJSON(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `[{"number":1},{"number":2}]`)
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (issueListCmd{}).Run([]string{}, ctx); err != nil {
		t.Fatal(err)
	}
	var out []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatalf("stdout is not JSON array: %v\n%s", err, ctx.Stdout.(*bytes.Buffer).String())
	}
	if len(out) != 2 {
		t.Errorf("len = %d", len(out))
	}

	// Same server, forced table format: padded header row plus separator.
	ctx = testCtx(ts)
	ctx.Format = cli.FormatTable
	if err := (issueListCmd{}).Run([]string{}, ctx); err != nil {
		t.Fatal(err)
	}
	got := ctx.Stdout.(*bytes.Buffer).String()
	if !strings.HasPrefix(got, "NUMBER") {
		t.Errorf("table output must start with NUMBER header, got %q", got)
	}
	lines := strings.Split(got, "\n")
	hasSep := false
	for _, l := range lines[1:] {
		if strings.HasPrefix(l, "---") {
			hasSep = true
			break
		}
	}
	if !hasSep {
		t.Errorf("table output must contain a dashed separator line, got %q", got)
	}
}

func TestIssueCommandsHaveHelpPages(t *testing.T) {
	for _, c := range IssueCommands() {
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

func TestIssueCloseAndOpen(t *testing.T) {
	var gotMethod, gotPath string
	var raw []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		raw, _ = io.ReadAll(r.Body)
		fmt.Fprint(w, `{"number":7,"state":"closed"}`)
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	stdout := ctx.Stdout.(*bytes.Buffer)

	if err := (issueStateCmd{closing: true}).Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/issues/7" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if string(raw) != `{"state":"closed"}` {
		t.Errorf("body = %q", raw)
	}
	out := stdout.String()
	if !strings.Contains(out, `"state": "closed"`) {
		t.Errorf("stdout = %q", out)
	}

	raw = nil
	stdout.Reset()
	if err := (issueStateCmd{}).Run([]string{"7"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/issues/7" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if string(raw) != `{"state":"open"}` {
		t.Errorf("body = %q", raw)
	}
	if !strings.Contains(stdout.String(), `"number": 7`) {
		t.Errorf("stdout = %q", stdout.String())
	}
}

func TestIssueCloseMaps404ToRuntimeExit(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		fmt.Fprint(w, `{"message":"Not Found"}`)
	}))
	defer ts.Close()

	err := (issueStateCmd{closing: true}).Run([]string{"99"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitRuntime {
		t.Fatalf("want ExitRuntime, got %v", err)
	}
}
