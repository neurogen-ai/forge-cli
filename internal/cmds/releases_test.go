package cmds

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
)

// ---- release list ----

func TestReleaseListJSONByDefault(t *testing.T) {
	var gotPath string
	pub := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		json.NewEncoder(w).Encode([]api.Release{{ID: 1, TagName: "v0.4.1", Name: "v0.4.1", PublishedAt: &pub}})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (releaseListCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/releases" {
		t.Errorf("path = %q", gotPath)
	}
	var rels []api.Release
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &rels); err != nil {
		t.Fatalf("stdout = %q: %v", ctx.Stdout.(*bytes.Buffer).String(), err)
	}
	if len(rels) != 1 || rels[0].TagName != "v0.4.1" {
		t.Errorf("releases = %+v", rels)
	}
}

func TestReleaseListTableColumns(t *testing.T) {
	pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]api.Release{
			{ID: 2, TagName: "v0.4.2", Name: "second", Draft: true},
			{ID: 1, TagName: "v0.4.1", Name: "first", Prerelease: true, PublishedAt: &pub},
		})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	ctx.Format = cli.FormatTable
	if err := (releaseListCmd{}).Run(nil, ctx); err != nil {
		t.Fatal(err)
	}
	out := ctx.Stdout.(*bytes.Buffer).String()
	for _, want := range []string{"TAG", "NAME", "DRAFT", "PRERELEASE", "PUBLISHED"} {
		if !strings.Contains(out, want) {
			t.Errorf("table missing column %q:\n%s", want, out)
		}
	}
	// Server order, not sorted; draft/prerelease rendered as truthy flags;
	// date rendered in the shared short form.
	if !strings.Contains(out, "v0.4.2") || strings.Contains(out, "v0.4.1") && strings.Index(out, "v0.4.2") > strings.Index(out, "v0.4.1") {
		t.Errorf("server order not preserved:\n%s", out)
	}
	if !strings.Contains(out, "2026-09-01") {
		t.Errorf("published date missing:\n%s", out)
	}
}

// ---- release get ----

func TestReleaseGetByTag(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1.0/beta", Name: "beta"})
	}))
	defer ts.Close()

	ctx := testCtx(ts)
	if err := (releaseGetCmd{}).Run([]string{"v1.0/beta"}, ctx); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/releases/v1.0%2Fbeta" {
		t.Errorf("path = %q, want escaped tag", gotPath)
	}
	var rel api.Release
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &rel); err != nil {
		t.Fatalf("stdout = %q: %v", ctx.Stdout.(*bytes.Buffer).String(), err)
	}
	if rel.TagName != "v1.0/beta" {
		t.Errorf("release = %+v", rel)
	}
}

func TestReleaseGetRequiresTag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request may be sent without a tag")
	}))
	defer ts.Close()
	ctx := testCtx(ts)

	if err := (releaseGetCmd{}).Run(nil, ctx); err == nil {
		t.Fatal("missing tag: want usage error, got nil")
	}
}

func TestReleaseCommandsRegistered(t *testing.T) {
	reg := cli.NewRegistry()
	reg.Register(ReleaseCommands()...)
	for _, name := range []string{"release list", "release get"} {
		if reg.Lookup(name) == nil {
			t.Errorf("command %q not registered", name)
		}
	}
}
