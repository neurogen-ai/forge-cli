package cmds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

func searchTestCtx(ts *httptest.Server, out *strings.Builder) *cli.Ctx {
	return &cli.Ctx{
		Stdout: out,
		Stderr: out,
		GlobalFlags: cli.GlobalFlags{
			Host: "git.example.com", Owner: "o", Repo: "r",
		},
		API: api.NewClient(ts.URL, "tok", 0, nil),
	}
}

func TestSearchKindRegistry(t *testing.T) {
	if searchKinds["issues"].kind != "issues" || searchKinds["prs"].kind != "pulls" {
		t.Fatalf("registry kinds = %q %q", searchKinds["issues"].kind, searchKinds["prs"].kind)
	}
	if len(searchKinds["issues"].columns) != len(issueListColumns) {
		t.Error("issues search must reuse issueListColumns")
	}
	if len(searchKinds["prs"].columns) != len(prListColumns) {
		t.Error("prs search must reuse prListColumns")
	}
	rows := searchKinds["prs"].rows([]api.Issue{{Number: 7, Title: "t", State: "open",
		User: api.User{Login: "u"}, CreatedAt: nil}})
	if len(rows) != 1 || rows[0][0] != "7" || rows[0][4] != "" {
		t.Errorf("prs rows = %v", rows)
	}
}

func TestSearchCommandParams(t *testing.T) {
	var gotPath, gotQ, gotType, gotState, gotPage, gotLimit string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQ = r.URL.Path, r.URL.Query().Get("q")
		gotType = r.URL.Query().Get("type")
		gotState = r.URL.Query().Get("state")
		gotPage = r.URL.Query().Get("page")
		gotLimit = r.URL.Query().Get("limit")
		json.NewEncoder(w).Encode([]api.Issue{})
	}))
	defer ts.Close()
	out := &strings.Builder{}
	ctx := searchTestCtx(ts, out)

	if err := (searchCmd{kind: "prs"}).Run([]string{"crash", "--state", "open", "--page", "2", "--limit", "5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues/search" {
		t.Errorf("path = %s", gotPath)
	}
	if gotQ != "crash" || gotType != "pulls" || gotState != "open" || gotPage != "2" || gotLimit != "5" {
		t.Errorf("params q=%q type=%q state=%q page=%q limit=%q", gotQ, gotType, gotState, gotPage, gotLimit)
	}
}

func TestSearchRequiresQuery(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer ts.Close()
	ctx := searchTestCtx(ts, &strings.Builder{})
	err := (searchCmd{kind: "issues"}).Run([]string{}, ctx)
	if err == nil {
		t.Fatal("expected usage error for missing QUERY")
	}
	if !strings.Contains(err.Error(), "requires exactly one QUERY") {
		t.Errorf("err = %v", err)
	}
}

func TestSearchJSONOutput(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]api.Issue{{Number: 3, Title: "hit"}})
	}))
	defer ts.Close()
	out := &strings.Builder{}
	ctx := searchTestCtx(ts, out)

	if err := (searchCmd{kind: "issues"}).Run([]string{"hit"}, ctx); err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal([]byte(out.String()), &got); err != nil {
		t.Fatalf("expected JSON array, got %q: %v", out.String(), err)
	}
	if len(got) != 1 || got[0]["number"].(float64) != 3 {
		t.Errorf("json = %v", got)
	}
}

func TestSearchCommandsRegistered(t *testing.T) {
	cmds := SearchCommands()
	if len(cmds) != 2 {
		t.Fatalf("got %d commands", len(cmds))
	}
	if cmds[0].Name() != "search issues" || cmds[1].Name() != "search prs" {
		t.Errorf("names = %q %q", cmds[0].Name(), cmds[1].Name())
	}
}
