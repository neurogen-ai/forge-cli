package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
