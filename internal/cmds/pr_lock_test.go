package cmds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"forge/internal/cli"
)

func TestPrLockLocksWithReason(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(204)
	}))
	defer ts.Close()

	out := &bytes.Buffer{}
	ctx := testCtx(ts)
	ctx.Stdout = out
	if err := (prLockCmd{locking: true}).Run([]string{"5", "--reason", "off-topic"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/issues/5/lock" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["reason"] != "off-topic" {
		t.Errorf("body = %v, want reason off-topic", gotBody)
	}
	var rc prLockReceipt
	if err := json.Unmarshal(out.Bytes(), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Index != 5 || rc.Action != "lock" || rc.Reason != "off-topic" {
		t.Errorf("receipt = %+v", rc)
	}
}

func TestPrLockNoReasonOmitsBodyAndReceiptReason(t *testing.T) {
	var gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw := make([]byte, 64)
		n, _ := r.Body.Read(raw)
		gotBody = string(raw[:n])
		w.WriteHeader(204)
	}))
	defer ts.Close()

	out := &bytes.Buffer{}
	ctx := testCtx(ts)
	ctx.Stdout = out
	if err := (prLockCmd{locking: true}).Run([]string{"5"}, ctx); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(gotBody) != "" {
		t.Errorf("body = %q, want empty", gotBody)
	}
	var rc prLockReceipt
	if err := json.Unmarshal(out.Bytes(), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Reason != "" || rc.Action != "lock" {
		t.Errorf("receipt = %+v", rc)
	}
}

func TestPrUnlock(t *testing.T) {
	var gotMethod, gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(204)
	}))
	defer ts.Close()

	out := &bytes.Buffer{}
	ctx := testCtx(ts)
	ctx.Stdout = out
	if err := (prLockCmd{locking: false}).Run([]string{"9"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/v1/repos/o/r/issues/9/lock" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	var rc prLockReceipt
	if err := json.Unmarshal(out.Bytes(), &rc); err != nil {
		t.Fatal(err)
	}
	if rc.Index != 9 || rc.Action != "unlock" {
		t.Errorf("receipt = %+v", rc)
	}
}

func TestPrLockBadIndexIsUsageError(t *testing.T) {
	err := (prLockCmd{locking: true}).Run([]string{"not-a-number"}, testCtx(httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {}))))
	if err == nil {
		t.Fatal("want error")
	}
	if cliErr, ok := err.(*cli.Error); !ok || cliErr.Code != cli.ExitUsage {
		t.Errorf("err = %v, want usage error", err)
	}
}

func TestPrLockServerErrorSurfacesVerbatim(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(403)
		w.Write([]byte(`{"message":"lock not supported"}`))
	}))
	defer ts.Close()

	err := (prLockCmd{locking: true}).Run([]string{"5", "--reason", "x"}, testCtx(ts))
	if err == nil || !strings.Contains(err.Error(), "lock not supported") {
		t.Errorf("err = %v, want server message verbatim", err)
	}
}

func TestPrLockRegistration(t *testing.T) {
	want := map[string]bool{"pr lock": false, "pr unlock": false}
	for _, c := range PRCommands() {
		if _, ok := want[c.Name()]; ok {
			want[c.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("%s not registered in PRCommands", name)
		}
	}
}
