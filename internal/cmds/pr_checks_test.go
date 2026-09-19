package cmds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/api"
	"forge/internal/cli"
)

func checksServer(t *testing.T, actionsStatus int) (*httptest.Server, *int) {
	t.Helper()
	taskHits := 0
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/5"):
			pr := map[string]any{"number": 5, "state": "open", "head": map[string]any{"ref": "feature", "sha": "abc123"}}
			json.NewEncoder(w).Encode(pr)
		case strings.HasSuffix(r.URL.Path, "/commits/abc123/statuses"):
			json.NewEncoder(w).Encode([]api.CommitStatus{{State: "success", Context: "ci/build", Description: "done"}})
		case strings.HasSuffix(r.URL.Path, "/actions/tasks"):
			taskHits++
			if actionsStatus != 200 {
				http.Error(w, `{"message":"Not Found"}`, actionsStatus)
				return
			}
			json.NewEncoder(w).Encode(map[string]any{
				"total_count": 1,
				"entries": []map[string]any{
					{"id": 7, "name": "build", "status": "running", "head_sha": "abc123", "head_branch": "feature", "url": "/o/r/actions/runs/7"},
				},
			})
		default:
			http.Error(w, `{"message":"unexpected"}`, 500)
		}
	}))
	return ts, &taskHits
}

func TestPRChecksTableAndNoteWhenActionsUnavailable(t *testing.T) {
	ts, _ := checksServer(t, 404)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatTable

	if err := (prChecksCmd{}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	note := ctx.Stderr.(*bytes.Buffer).String()
	if !strings.Contains(note, "note: Actions not available on this instance") {
		t.Errorf("stderr = %q", note)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"status/..", "ci/build"} {
		if !strings.Contains(out, want) {
			t.Errorf("table output %q missing %q", out, want)
		}
	}
}

func TestPRChecksMergesStatusesAndRuns(t *testing.T) {
	ts, _ := checksServer(t, 200)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatTable

	if err := (prChecksCmd{}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ctx.Stderr.(*bytes.Buffer).String(), "note:") {
		t.Errorf("unexpected note on stderr: %q", ctx.Stderr.(*bytes.Buffer).String())
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"status/..", "run/run", "build"} {
		if !strings.Contains(out, want) {
			t.Errorf("output %q missing %q", out, want)
		}
	}
	// statuses before runs in the merged view
	if strings.Index(out, "ci/build") > strings.Index(out, "Actions run #7") {
		t.Errorf("statuses must precede runs in %q", out)
	}
}

func TestPRChecksJSON(t *testing.T) {
	ts, _ := checksServer(t, 200)
	defer ts.Close()
	ctx := testCtx(ts)
	ctx.Format = cli.FormatJSON

	if err := (prChecksCmd{}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Ref              string             `json:"ref"`
		Statuses         []api.CommitStatus `json:"statuses"`
		Runs             []api.ActionRun    `json:"runs"`
		ActionsAvailable bool               `json:"actions_available"`
	}
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &payload); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, ctx.Stdout.(*bytes.Buffer).String())
	}
	if payload.Ref != "abc123" || !payload.ActionsAvailable {
		t.Errorf("payload = %+v", payload)
	}
	if len(payload.Statuses) != 1 || len(payload.Runs) != 1 {
		t.Errorf("statuses=%d runs=%d", len(payload.Statuses), len(payload.Runs))
	}
}

func TestPRChecksErrorsOnBadIndex(t *testing.T) {
	ts, _ := checksServer(t, 200)
	defer ts.Close()
	ctx := testCtx(ts)
	if err := (prChecksCmd{}).Run([]string{"abc"}, ctx); err == nil {
		t.Error("expected usage error for non-numeric index")
	}
}

func TestPRChecksErrorOnStatusesFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/pulls/5"):
			json.NewEncoder(w).Encode(map[string]any{"head": map[string]any{"ref": "x", "sha": "abc"}})
		default:
			http.Error(w, `{"message":"boom"}`, 500)
		}
	}))
	defer ts.Close()
	ctx := testCtx(ts)
	err := (prChecksCmd{}).Run([]string{"5"}, ctx)
	if err == nil {
		t.Fatal("expected error")
	}
	if ec, ok := err.(*cli.Error); !ok || ec.Code == cli.ExitOK {
		t.Errorf("expected nonzero exit code, got %v", err)
	}
}
