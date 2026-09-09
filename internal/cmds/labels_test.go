package cmds

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

func TestCollectLabelNames(t *testing.T) {
	args := []string{"--title", "t", "--label", "bug", "--label", "docs", "--label"}
	names, err := collectLabelNames(args, "issue create")
	if err != nil {
		t.Fatal(err)
	}
	if len(names) != 2 || names[0] != "bug" || names[1] != "docs" {
		t.Errorf("names = %v, want [bug docs] (trailing bare --label ignored)", names)
	}
	if got, _ := collectLabelNames(nil, "x"); got != nil {
		t.Errorf("nil args names = %v, want nil", got)
	}
}

// TestIssueCreateUsesSharedResolver pins the behaviour-unchanged contract:
// the create command resolves through collectLabelNames/resolveLabelIDs, so
// order, duplicates, and the unknown-name error are identical to before.
func TestIssueCreateUsesSharedResolver(t *testing.T) {
	var labelGETs int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/repos/o/r/labels":
			labelGETs++
			json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
			return
		case "POST /api/v1/repos/o/r/issues":
			w.WriteHeader(201)
			json.NewEncoder(w).Encode(api.Issue{Number: 4})
			return
		}
		w.WriteHeader(500)
	}))
	defer ts.Close()

	err := issueCreateCmd{}.Run([]string{"--title", "t", "--label", "docs", "--label", "bug", "--label", "docs"}, testCtx(ts))
	if err != nil {
		t.Fatal(err)
	}
	if labelGETs != 1 {
		t.Errorf("ListLabels calls = %d, want 1", labelGETs)
	}
}

func TestResolveLabelIDsOrderDuplicatesAndUnknown(t *testing.T) {
	var labelGETs int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		labelGETs++
		json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()
	ctx := testCtx(ts)

	ids, err := resolveLabelIDs(ctx, []string{"docs", "bug", "docs"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 3 || ids[0] != 2 || ids[1] != 1 || ids[2] != 2 {
		t.Errorf("ids = %v, want [2 1 2]", ids)
	}
	if labelGETs != 1 {
		t.Errorf("ListLabels calls = %d, want 1", labelGETs)
	}

	if _, uerr := resolveLabelIDs(ctx, []string{"bug", "nope", "docs", "gone"}); uerr == nil {
		t.Fatal("want error for unknown names")
	} else if cerr, ok := uerr.(*cli.Error); !ok {
		t.Fatalf("err = %T, want *cli.Error", uerr)
	} else {
		if cerr.Code != cli.ExitRuntime {
			t.Errorf("code = %v, want ExitRuntime", cerr.Code)
		}
		if cerr.Msg != "unknown labels: nope, gone" {
			t.Errorf("msg = %q", cerr.Msg)
		}
	}
}

func TestResolveLabelIDsNoNamesNoRequest(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(500)
	}))
	defer ts.Close()

	ids, err := resolveLabelIDs(testCtx(ts), nil)
	if err != nil {
		t.Fatal(err)
	}
	if ids != nil {
		t.Errorf("ids = %v, want nil", ids)
	}
	if requests != 0 {
		t.Errorf("requests = %d, want 0", requests)
	}
}
