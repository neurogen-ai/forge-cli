package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

func TestListReleasesPathOrderAndAuth(t *testing.T) {
	var gotPath, gotAuth string
	var calls int
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		gotPath, gotAuth = r.URL.Path, r.Header.Get("Authorization")
		if gotAuth != "token tok" {
			t.Errorf("Authorization = %q", gotAuth)
		}
		pub := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
		json.NewEncoder(w).Encode([]Release{
			{ID: 2, TagName: "v0.2.0", Name: "second", PublishedAt: &pub},
			{ID: 1, TagName: "v0.1.0", Name: "first"},
		})
	}))
	defer ts.Close()

	rels, err := newTestClient(ts).ListReleases("o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 {
		t.Errorf("calls = %d, want 1", calls)
	}
	if gotPath != "/api/v1/repos/o/r/releases" {
		t.Errorf("path = %q", gotPath)
	}
	// Server order is preserved, not sorted or filtered.
	if len(rels) != 2 || rels[0].TagName != "v0.2.0" || rels[1].TagName != "v0.1.0" {
		t.Errorf("releases = %+v", rels)
	}
	if rels[0].PublishedAt == nil || !rels[0].PublishedAt.Equal(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("PublishedAt = %v", rels[0].PublishedAt)
	}
	if len(rels[1].Assets) != 0 {
		t.Errorf("absent assets must decode as empty slice, got %v", rels[1].Assets)
	}
}

func TestListReleasesFollowsPagination(t *testing.T) {
	var srvURL string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Content-Type", "application/json")
		if page < 1 {
			w.Header().Set("Link", `<`+srvURL+`?page=`+strconv.Itoa(page+1)+`>; rel="next"`)
			json.NewEncoder(w).Encode([]Release{{ID: 1, TagName: "v0.1.0"}})
			return
		}
		json.NewEncoder(w).Encode([]Release{{ID: 2, TagName: "v0.2.0"}})
	}))
	defer ts.Close()
	srvURL = ts.URL + "/api/v1/repos/o/r/releases"

	rels, err := newTestClient(ts).ListReleases("o", "r")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 || rels[0].TagName != "v0.1.0" || rels[1].TagName != "v0.2.0" {
		t.Errorf("releases = %+v", rels)
	}
}

func TestListReleasesSurfacesAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"no releases here"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).ListReleases("o", "gone")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusNotFound || apiErr.Message != "no releases here" {
		t.Fatalf("err = %v (%T), want *APIError{404 no releases here}", err, err)
	}
}

func TestGetReleasePathEscapesTag(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		json.NewEncoder(w).Encode(Release{
			ID:      3,
			TagName: "v1.0/beta",
			Assets: []ReleaseAsset{
				{ID: 9, Name: "forge.tgz", Size: 1024, BrowserDownloadURL: "https://h/dl/forge.tgz"},
			},
		})
	}))
	defer ts.Close()

	rel, err := newTestClient(ts).GetRelease("o", "r", "v1.0/beta")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/releases/v1.0%2Fbeta" {
		t.Errorf("path = %q, want escaped tag", gotPath)
	}
	if rel.TagName != "v1.0/beta" || len(rel.Assets) != 1 || rel.Assets[0].BrowserDownloadURL != "https://h/dl/forge.tgz" {
		t.Errorf("release = %+v", rel)
	}
}

func TestGetReleaseSurfacesAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"no such tag"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).GetRelease("o", "r", "nope")
	apiErr, ok := err.(*APIError)
	if !ok || apiErr.Status != http.StatusNotFound || apiErr.Message != "no such tag" {
		t.Fatalf("err = %v (%T), want *APIError{404 no such tag}", err, err)
	}
}
