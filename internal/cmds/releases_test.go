package cmds

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"forge/internal/api"
	"forge/internal/cli"
	"forge/internal/config"
	"forge/internal/gitctx"
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

// ---- release download ----

// downloadCtx adds the repo root and the seeded releases savedir.
func downloadCtx(ts *httptest.Server, root string) *cli.Ctx {
	ctx := testCtx(ts)
	ctx.Cfg = &config.Config{Savedirs: map[string]string{"releases": ".forge/cache/releases"}}
	ctx.Repo = &gitctx.Repo{Root: root}
	return ctx
}

func TestReleaseDownloadAllAssets(t *testing.T) {
	var downloads []string
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/o/r/releases/v0.4.2":
			json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v0.4.2", Assets: []api.ReleaseAsset{
				{ID: 1, Name: "forge-linux.tgz", BrowserDownloadURL: ts.URL + "/dl/forge-linux.tgz"},
				{ID: 2, Name: "checksums.txt", BrowserDownloadURL: ts.URL + "/dl/checksums.txt"},
			}})
		case r.URL.Path == "/dl/forge-linux.tgz":
			downloads = append(downloads, r.URL.Path)
			if r.Header.Get("Authorization") != "token tok" {
				t.Errorf("Authorization = %q", r.Header.Get("Authorization"))
			}
			w.Write([]byte{0x1F, 0x8B, 0x00})
		case r.URL.Path == "/dl/checksums.txt":
			downloads = append(downloads, r.URL.Path)
			fmt.Fprint(w, "abc  forge-linux.tgz\n")
		default:
			t.Errorf("unexpected path %q", r.URL.Path)
		}
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	if err := (releaseDownloadCmd{}).Run([]string{"v0.4.2"}, ctx); err != nil {
		t.Fatal(err)
	}
	if len(downloads) != 2 {
		t.Errorf("downloads = %v, want 2 sequential fetches", downloads)
	}
	var receipt ReleaseDownloadReceipt
	if err := json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt); err != nil {
		t.Fatalf("stdout not a receipt: %v", err)
	}
	if receipt.Tag != "v0.4.2" || len(receipt.Files) != 2 {
		t.Fatalf("receipt = %+v", receipt)
	}
	if receipt.Files[0].Name != "forge-linux.tgz" || receipt.Files[0].Bytes != 3 {
		t.Errorf("file 0 = %+v", receipt.Files[0])
	}
	if !strings.HasSuffix(receipt.Files[0].Path, filepath.Join(".forge", "cache", "releases", "forge-linux.tgz")) {
		t.Errorf("path = %q", receipt.Files[0].Path)
	}
	data, err := os.ReadFile(filepath.Join(root, ".forge", "cache", "releases", "checksums.txt"))
	if err != nil || string(data) != "abc  forge-linux.tgz\n" {
		t.Errorf("checksums.txt = %q, err %v", data, err)
	}
}

func TestReleaseDownloadAssetSubsetInInputOrder(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/o/r/releases/v1":
			json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1", Assets: []api.ReleaseAsset{
				{ID: 1, Name: "a.zip", BrowserDownloadURL: ts.URL + "/dl/a.zip"},
				{ID: 2, Name: "b.zip", BrowserDownloadURL: ts.URL + "/dl/b.zip"},
			}})
		case r.URL.Path == "/dl/a.zip":
			fmt.Fprint(w, "aaa")
		case r.URL.Path == "/dl/b.zip":
			fmt.Fprint(w, "bbb")
		}
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	if err := (releaseDownloadCmd{}).Run([]string{"v1", "--asset", "b.zip", "--asset", "a.zip"}, ctx); err != nil {
		t.Fatal(err)
	}
	var receipt ReleaseDownloadReceipt
	json.Unmarshal(ctx.Stdout.(*bytes.Buffer).Bytes(), &receipt)
	if len(receipt.Files) != 2 || receipt.Files[0].Name != "b.zip" || receipt.Files[1].Name != "a.zip" {
		t.Errorf("receipt order = %+v, want input order", receipt.Files)
	}
	if _, err := os.Stat(filepath.Join(root, ".forge", "cache", "releases", "a.zip")); err != nil {
		t.Errorf("a.zip missing: %v", err)
	}
}

func TestReleaseDownloadOverwritesPerFilename(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/o/r/releases/v1":
			json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1", Assets: []api.ReleaseAsset{
				{ID: 1, Name: "a.zip", BrowserDownloadURL: ts.URL + "/dl/a.zip"},
			}})
		case r.URL.Path == "/dl/a.zip":
			fmt.Fprint(w, "new-bytes")
		}
	}))
	defer ts.Close()

	root := t.TempDir()
	target := filepath.Join(root, ".forge", "cache", "releases", "a.zip")
	os.MkdirAll(filepath.Dir(target), 0o755)
	os.WriteFile(target, []byte("stale"), 0o644)

	if err := (releaseDownloadCmd{}).Run([]string{"v1"}, downloadCtx(ts, root)); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "new-bytes" {
		t.Errorf("a.zip = %q, err %v; want overwrite", data, err)
	}
}

func TestReleaseDownloadUnknownAssetStopsBeforeAnyDownload(t *testing.T) {
	var downloads int
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/repos/o/r/releases/v1" {
			json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1", Assets: []api.ReleaseAsset{
				{ID: 1, Name: "a.zip", BrowserDownloadURL: ts.URL + "/dl/a.zip"},
			}})
			return
		}
		downloads++
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	err := (releaseDownloadCmd{}).Run([]string{"v1", "--asset", "nope.zip"}, ctx)
	if err == nil || !strings.Contains(err.Error(), "no asset nope.zip") {
		t.Fatalf("err = %v, want unknown-asset runtime error", err)
	}
	if downloads != 0 || ctx.Stdout.(*bytes.Buffer).Len() != 0 {
		t.Errorf("downloads = %d, stdout = %q; nothing may download or print", downloads, ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestReleaseDownloadMidBatchFailureKeepsFilesPrintsNoReceipt(t *testing.T) {
	var ts *httptest.Server
	ts = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api/v1/repos/o/r/releases/v1":
			json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1", Assets: []api.ReleaseAsset{
				{ID: 1, Name: "good.zip", BrowserDownloadURL: ts.URL + "/dl/good.zip"},
				{ID: 2, Name: "bad.zip", BrowserDownloadURL: ts.URL + "/dl/bad.zip"},
			}})
		case r.URL.Path == "/dl/good.zip":
			fmt.Fprint(w, "good")
		case r.URL.Path == "/dl/bad.zip":
			w.WriteHeader(http.StatusNotFound)
			fmt.Fprint(w, `{"message":"asset gone"}`)
		}
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	err := (releaseDownloadCmd{}).Run([]string{"v1"}, ctx)
	if err == nil {
		t.Fatal("mid-batch failure: want error, got nil")
	}
	data, rerr := os.ReadFile(filepath.Join(root, ".forge", "cache", "releases", "good.zip"))
	if rerr != nil || string(data) != "good" {
		t.Errorf("good.zip = %q, err %v; earlier files stay", data, rerr)
	}
	if _, serr := os.Stat(filepath.Join(root, ".forge", "cache", "releases", "bad.zip")); serr == nil {
		t.Error("bad.zip must not be written")
	}
	if ctx.Stdout.(*bytes.Buffer).Len() != 0 {
		t.Errorf("stdout = %q; no receipt on failure", ctx.Stdout.(*bytes.Buffer).String())
	}
}

func TestReleaseDownloadAssetLessReleaseReceiptsEmptyFiles(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"id":1,"tag_name":"v1","assets":[]}`)
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	if err := (releaseDownloadCmd{}).Run([]string{"v1"}, ctx); err != nil {
		t.Fatal(err)
	}
	if out := ctx.Stdout.(*bytes.Buffer).String(); !strings.Contains(out, `"files": []`) {
		t.Errorf("stdout = %q; asset-less receipt must serialize files: []", out)
	}
}

func TestReleaseDownloadOffHostAssetRefuses(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("token must never reach an off-host asset")
	}))
	defer evil.Close()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(api.Release{ID: 1, TagName: "v1", Assets: []api.ReleaseAsset{
			{ID: 1, Name: "x.zip", BrowserDownloadURL: evil.URL + "/x.zip"},
		}})
	}))
	defer ts.Close()

	root := t.TempDir()
	ctx := downloadCtx(ts, root)
	if err := (releaseDownloadCmd{}).Run([]string{"v1"}, ctx); err == nil {
		t.Fatal("off-host asset: want error, got nil")
	}
	if _, serr := os.Stat(filepath.Join(root, ".forge", "cache", "releases", "x.zip")); serr == nil {
		t.Error("x.zip must not be written")
	}
}

func TestReleaseDownloadRequiresTag(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request may be sent without a tag")
	}))
	defer ts.Close()

	if err := (releaseDownloadCmd{}).Run(nil, downloadCtx(ts, t.TempDir())); err == nil {
		t.Fatal("missing tag: want usage error, got nil")
	}
}
