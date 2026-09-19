package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

// fakeSection records Fetch calls and returns canned entries, so the
// dashboard loop's order-once contract is testable without an API client.
type fakeSection struct {
	title   string
	entries []statusEntry
	calls   int
	fail    bool
}

func (f *fakeSection) Title() string { return f.title }
func (f *fakeSection) Fetch(ctx *cli.Ctx, me *api.User) ([]statusEntry, error) {
	f.calls++
	if f.fail {
		return nil, fmt.Errorf("boom")
	}
	return f.entries, nil
}

func TestStatusDashboardFetchesSectionsInRegistrationOrder(t *testing.T) {
	first := &fakeSection{title: "one", entries: []statusEntry{{Kind: "pr", Index: 7, Title: "t7", State: "open", Reason: "assigned"}}}
	second := &fakeSection{title: "two", entries: []statusEntry{{Kind: "issue", Index: 3, Title: "t3", State: "open", Reason: "open"}}}
	empty := &fakeSection{title: "empty"}

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/user" {
			t.Errorf("path = %s, want /api/v1/user", r.URL.Path)
		}
		fmt.Fprint(w, `{"id":1,"login":"me"}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatJSON

	if err := runStatusDashboard(ctx, []statusSection{first, empty, second}); err != nil {
		t.Fatal(err)
	}
	// Every section is fetched exactly once, in registration order, even the
	// empty one.
	if first.calls != 1 || second.calls != 1 || empty.calls != 1 {
		t.Errorf("calls = %d %d %d, want 1 each", first.calls, second.calls, empty.calls)
	}
	var got []statusSectionOut
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &got); err != nil {
		t.Fatalf("stdout %q: %v", ctx.Stdout.(*bytes.Buffer).String(), err)
	}
	if len(got) != 2 {
		t.Fatalf("sections = %d, want 2 (empty skipped)", len(got))
	}
	if got[0].Title != "one" || got[1].Title != "two" {
		t.Errorf("order = %q %q, want one then two", got[0].Title, got[1].Title)
	}
	if got[0].Entries[0].Index != 7 || got[1].Entries[0].Reason != "open" {
		t.Errorf("entries = %+v", got)
	}
}

func TestStatusDashboardCurrentUserErrorSurfaces(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		fmt.Fprint(w, `{"message":"bad token"}`)
	}))
	defer ts.Close()
	ctx := testCtx(ts)

	if err := runStatusDashboard(ctx, []statusSection{&fakeSection{title: "x"}}); err == nil {
		t.Fatal("want an error when CurrentUser fails")
	}
}

func TestStatusCommandRegisters(t *testing.T) {
	sc := StatusCommand()
	if sc.Name() != "status" {
		t.Errorf("name = %q", sc.Name())
	}
	if !sc.(interface{ RequiresAPI() bool }).RequiresAPI() {
		t.Error("status must require the API client")
	}
}

// TestStatusPRSections pins both PR sections and the shared-fetch rule: one
// dashboard run hits /pulls exactly once even though two sections read it.
func TestStatusPRSections(t *testing.T) {
	var pullHits int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/v1/user":
			fmt.Fprint(w, `{"id":1,"login":"me"}`)
		case "/api/v1/repos/o/r/pulls":
			pullHits++
			fmt.Fprint(w, `[
				{"number":7,"title":"fix leak","state":"open",
				 "requested_reviewers":[{"login":"me"}],
				 "html_url":"https://x/7"},
				{"number":9,"title":"add cache","state":"open",
				 "assignees":[{"login":"me"}],
				 "html_url":"https://x/9"},
				{"number":11,"title":"other","state":"open",
				 "requested_reviewers":[{"login":"you"}],"assignees":[{"login":"you"}]}
			]`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatJSON

	if err := runStatusDashboard(ctx, statusSections()); err != nil {
		t.Fatal(err)
	}
	if pullHits != 1 {
		t.Fatalf("pulls hits = %d, want exactly 1", pullHits)
	}
	var got []statusSectionOut
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("sections = %d, want 2", len(got))
	}
	if got[0].Title != "Review requests awaiting you" || got[0].Entries[0].Index != 7 ||
		got[0].Entries[0].Reason != "review-requested" {
		t.Errorf("section 0 = %+v", got[0])
	}
	if got[1].Title != "Pull requests assigned to you" || got[1].Entries[0].Index != 9 ||
		got[1].Entries[0].Reason != "assigned" {
		t.Errorf("section 1 = %+v", got[1])
	}
}

// TestPullRequestDecodesAssignees pins the subset extension: Forgejo's
// assignees array decodes onto the PullRequest read type.
func TestPullRequestDecodesAssignees(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"number":9,"assignees":[{"id":2,"login":"me"}]}`)
	}))
	defer ts.Close()
	pr, err := api.NewClient(ts.URL, "t", 0, nil).GetPullRequest("o", "r", 9)
	if err != nil {
		t.Fatal(err)
	}
	if len(pr.Assignees) != 1 || pr.Assignees[0].Login != "me" {
		t.Errorf("assignees = %+v", pr.Assignees)
	}
}
