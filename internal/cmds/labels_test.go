package cmds

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
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

// ---- label list ----

func TestLabelListJSONByDefault(t *testing.T) {
	var gotPath, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		json.NewEncoder(w).Encode([]api.Label{{ID: 3, Name: "bug", Color: "ee0701"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (labelListCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/labels" || gotMethod != "GET" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	var labels []api.Label
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &labels); err != nil {
		t.Fatalf("stdout = %q: %v", ctx.Stdout.(*bytes.Buffer).String(), err)
	}
	if len(labels) != 1 || labels[0].ID != 3 || labels[0].Name != "bug" || labels[0].Color != "ee0701" {
		t.Errorf("labels = %+v", labels)
	}
}

func TestLabelListTable(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]api.Label{{ID: 3, Name: "bug", Color: "ee0701"}, {ID: 7, Name: "docs", Color: "c5def5"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	ctx.Format = cli.FormatTable
	if err := (labelListCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"NAME", "COLOR", "ID", "bug", "ee0701", "7"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output missing %q in %q", want, out)
		}
	}
}

func TestLabelListRows(t *testing.T) {
	rows := labelListRows([]api.Label{{ID: 3, Name: "bug", Color: "ee0701"}})
	if len(rows) != 1 || rows[0][0] != "bug" || rows[0][1] != "ee0701" || rows[0][2] != "3" {
		t.Errorf("rows = %v", rows)
	}
	if got := labelListRows(nil); len(got) != 0 {
		t.Errorf("nil rows = %v", got)
	}
}

// ---- issue label add / remove ----

func TestIssueLabelAddOnePOSTWithAllIDs(t *testing.T) {
	var calls []string
	var addBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Method == "POST" {
			raw, _ := io.ReadAll(r.Body)
			addBody = string(raw)
			json.NewEncoder(w).Encode([]api.Label{{ID: 2}, {ID: 1}})
			return
		}
		json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (issueLabelCmd{adding: true}).Run([]string{"5", "--label", "docs", "--label", "bug"}, ctx); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "GET /api/v1/repos/o/r/labels" || calls[1] != "POST /api/v1/repos/o/r/issues/5/labels" {
		t.Errorf("calls = %v", calls)
	}
	if addBody != `{"labels":[2,1]}` {
		t.Errorf("body = %q", addBody)
	}
	var receipt LabelReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatalf("receipt = %q: %v", ctx.Stdout.(*bytes.Buffer).String(), err)
	}
	if receipt.Index != 5 || receipt.Action != "add" || len(receipt.Labels) != 2 || receipt.Labels[0] != "docs" || receipt.Labels[1] != "bug" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestIssueLabelRemoveDeletesPerIDInOrder(t *testing.T) {
	var deletes []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes = append(deletes, r.URL.Path)
			w.WriteHeader(204)
			return
		}
		json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (issueLabelCmd{adding: false}).Run([]string{"5", "--label", "docs", "--label", "bug"}, ctx); err != nil {
		t.Fatal(err)
	}
	if len(deletes) != 2 || deletes[0] != "/api/v1/repos/o/r/issues/5/labels/2" || deletes[1] != "/api/v1/repos/o/r/issues/5/labels/1" {
		t.Errorf("deletes = %v", deletes)
	}
	var receipt LabelReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Action != "remove" || len(receipt.Labels) != 2 || receipt.Labels[0] != "docs" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestIssueLabelRemoveStopsOnFirstFailure(t *testing.T) {
	var deletes []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "DELETE" {
			deletes = append(deletes, r.URL.Path)
			w.WriteHeader(500)
			w.Write([]byte(`{"message":"label does not exist"}`))
			return
		}
		json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	err := (issueLabelCmd{adding: false}).Run([]string{"5", "--label", "docs", "--label", "bug"}, ctx)
	if _, ok := err.(*cli.Error); !ok {
		t.Fatalf("err = %T, want *cli.Error", err)
	}
	if len(deletes) != 1 || !strings.Contains(deletes[0], "/labels/2") {
		t.Errorf("deletes = %v, want exactly the first removal", deletes)
	}
	if ctx.Stdout.(*bytes.Buffer).Len() != 0 {
		t.Errorf("receipt printed on failure: %q", ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestIssueLabelUnknownNameNoMutation(t *testing.T) {
	var mutations []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			mutations = append(mutations, r.Method+" "+r.URL.Path)
		}
		json.NewEncoder(w).Encode([]api.Label{{ID: 1, Name: "bug"}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	err := (issueLabelCmd{adding: true}).Run([]string{"5", "--label", "nope"}, ctx)
	cerr, ok := err.(*cli.Error)
	if !ok {
		t.Fatalf("err = %T, want *cli.Error", err)
	}
	if cerr.Code != cli.ExitRuntime || cerr.Msg != "unknown labels: nope" {
		t.Errorf("err = %+v", cerr)
	}
	if len(mutations) != 0 || ctx.Stdout.(*bytes.Buffer).Len() != 0 {
		t.Errorf("mutations = %v, stdout = %q", mutations, ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestIssueLabelRequiresLabelFlag(t *testing.T) {
	requests := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(500)
	}))
	defer ts.Close()

	for _, cmd := range []issueLabelCmd{{adding: true}, {adding: false}} {
		err := cmd.Run([]string{"5"}, testCtx(ts))
		cerr, ok := err.(*cli.Error)
		if !ok {
			t.Fatalf("%s: err = %T, want *cli.Error", cmd.Name(), err)
		}
		if cerr.Code != cli.ExitUsage || cerr.Msg != cmd.Name()+" requires at least one --label NAME" {
			t.Errorf("%s: err = %+v", cmd.Name(), cerr)
		}
	}
	if requests != 0 {
		t.Errorf("requests = %d, want 0", requests)
	}
}

func TestLabelCommandsRegistered(t *testing.T) {
	reg := cli.NewRegistry()
	reg.Register(LabelCommands()...)
	for _, name := range []string{"label list", "issue label add", "issue label remove"} {
		if reg.Lookup(name) == nil {
			t.Errorf("%q not registered", name)
		}
	}
}
