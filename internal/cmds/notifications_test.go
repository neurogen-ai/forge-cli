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

func TestNotificationsListPassesAllFlag(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]api.Notification{{ID: 3, Unread: true}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	if err := (notificationListCmd{}).Run([]string{"--all"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "all=true" {
		t.Errorf("query = %q, want all=true", gotQuery)
	}
	var out []map[string]any
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &out); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if len(out) != 1 {
		t.Errorf("out = %v", out)
	}
}

func TestNotificationsListTableOnTTY(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]api.Notification{
			{ID: 1, Unread: true, Subject: struct {
				Title string `json:"title"`
				Type  string `json:"type"`
				URL   string `json:"url"`
			}{Title: "fix the leak", Type: "PullRequest", URL: "http://x/1"}, Repository: api.Repository{FullName: "o/r"}},
		})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	if td, ok := any(notificationListCmd{}).(interface{ DefaultIsTable() bool }); !ok || !td.DefaultIsTable() {
		t.Fatal("notifications must default to table on TTY")
	}
	_ = ctx
}

func TestNotificationsListNeverMarksRead(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "GET" {
			t.Errorf("listing issued %s request; only GET is allowed", r.Method)
		}
		json.NewEncoder(w).Encode([]api.Notification{})
	}))
	defer ts.Close()

	if err := (notificationListCmd{}).Run(nil, testCtx(ts)); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationsReadSendsBatchAndPrintsReceipt(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(205)
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	ctx.Stdout = &bytes.Buffer{}
	if err := (notificationsReadCmd{}).Run([]string{"42", "57"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PUT" || gotPath != "/api/v1/notifications" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	ids, _ := gotBody["ids"].([]any)
	if len(ids) != 2 || ids[0] != float64(42) || ids[1] != float64(57) {
		t.Errorf("body = %v", gotBody)
	}
	var receipt struct {
		IDs    []int64 `json:"ids"`
		Action string  `json:"action"`
	}
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatal(err)
	}
	if len(receipt.IDs) != 2 || receipt.IDs[0] != 42 || receipt.IDs[1] != 57 || receipt.Action != "read" {
		t.Errorf("receipt = %+v", receipt)
	}
}

func TestNotificationsReadNoIDsIsUsageWithoutRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request for zero ids")
	}))
	defer ts.Close()

	err := (notificationsReadCmd{}).Run(nil, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
	if !strings.Contains(cerr.Msg, "one or more IDs") {
		t.Errorf("msg = %q", cerr.Msg)
	}
}

func TestNotificationsReadRejectsNonNumericIDBeforeRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request for invalid id")
	}))
	defer ts.Close()

	err := (notificationsReadCmd{}).Run([]string{"abc"}, testCtx(ts))
	cerr, ok := err.(*cli.Error)
	if !ok || cerr.Code != cli.ExitUsage {
		t.Fatalf("want ExitUsage, got %v", err)
	}
}
