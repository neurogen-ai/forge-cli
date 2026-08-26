package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"
)

func TestPageParams(t *testing.T) {
	tests := []struct {
		name  string
		state string
		page  int
		limit int
		want  url.Values
	}{
		{"all empty", "", 0, 0, url.Values{}},
		{"state only", "open", 0, 0, url.Values{"state": {"open"}}},
		{"page and limit", "", 2, 30, url.Values{"page": {"2"}, "limit": {"30"}}},
		{"everything", "closed", 3, 10, url.Values{"state": {"closed"}, "page": {"3"}, "limit": {"10"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := PageParams(tt.state, tt.page, tt.limit)
			for k, v := range tt.want {
				if gotValues := got[k]; len(gotValues) != len(v) || gotValues[0] != v[0] {
					t.Errorf("PageParams(%q,%d,%d)[%s] = %v, want %v", tt.state, tt.page, tt.limit, k, gotValues, v)
				}
				delete(got, k)
			}
			for k := range got {
				t.Errorf("PageParams(%q,%d,%d): unexpected key %q", tt.state, tt.page, tt.limit, k)
			}
		})
	}
}

func TestNextLink(t *testing.T) {
	tests := []struct {
		name   string
		header string
		want   string
		ok     bool
	}{
		{
			"quoted rel",
			`<https://host/api/v1/x?page=2>; rel="next", <https://host/api/v1/x?page=1>; rel="prev"`,
			"https://host/api/v1/x?page=2",
			true,
		},
		{"unquoted rel", `<https://host/n?o=2>; rel=next`, "https://host/n?o=2", true},
		{"only prev", `</p>; rel="prev"`, "", false},
		{"no header", "", "", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			resp := &http.Response{Header: http.Header{}}
			if tt.header != "" {
				resp.Header.Set("Link", tt.header)
			}
			got, ok := NextLink(resp)
			if ok != tt.ok || got != tt.want {
				t.Errorf("NextLink() = %q,%v; want %q,%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestListFollowsNextLinksAcrossPages(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "token sekrit" {
			t.Errorf("Authorization = %q, want %q", got, "token sekrit")
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Query().Get("page") == "" || r.URL.Query().Get("page") == "1" {
			w.Header().Set("Link", `<`+srv.URL+`/api/v1/things?page=2>; rel="next"`)
			w.Write([]byte(`[1,2]`))
			return
		}
		w.Write([]byte(`[3]`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "sekrit", 5*time.Second, nil)
	got, err := List[int](client, "/things", url.Values{"page": {"1"}, "limit": {"2"}})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	want := []int{1, 2, 3}
	if len(got) != len(want) {
		t.Fatalf("List returned %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("List returned %v, want %v", got, want)
		}
	}
}

func TestListSurfacesAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"message":"gone"}`))
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "", time.Second, nil)
	if _, err := List[int](client, "/things", nil); err == nil {
		t.Fatal("List on 404: want error, got nil")
	} else if apiErr, ok := err.(*APIError); !ok || apiErr.Status != http.StatusNotFound || apiErr.Message != "gone" {
		t.Fatalf("List on 404 = %v (%T), want *APIError{404 gone}", err, err)
	}
}

func TestListRejectsPaginationLoop(t *testing.T) {
	var srvURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page, _ := strconv.Atoi(r.URL.Query().Get("page"))
		w.Header().Set("Link", `<`+srvURL+`?page=`+strconv.Itoa(page+1)+`>; rel="next"`)
		w.Write([]byte(`[]`))
	}))
	defer srv.Close()
	srvURL = srv.URL + "/api/v1/things"

	client := NewClient(srv.URL, "", time.Second, nil)
	if _, err := List[int](client, "/things", nil); err == nil {
		t.Fatal("List with self-perpetuating next links: want error, got nil")
	}
}

func TestTypesDecodeForgejoFieldNames(t *testing.T) {
	const body = `{
	  "number": 7, "title": "t", "body": "b", "state": "open",
	  "user": {"id": 1, "login": "amy"},
	  "html_url": "https://h/o/r/pull/7", "created_at": "2026-08-26T00:00:00Z",
	  "head": {"ref": "feat", "sha": "abc"}, "base": {"ref": "main"},
	  "labels": [{"id": 3, "name": "bug", "color": "#ff0000"}]
	}`
	var pr PullRequest
	if err := json.Unmarshal([]byte(body), &pr); err != nil {
		t.Fatalf("decode PullRequest: %v", err)
	}
	if pr.Number != 7 || pr.Head.Ref != "feat" || pr.Head.Sha != "abc" || pr.Base.Ref != "main" ||
		pr.User.Login != "amy" || len(pr.Labels) != 1 || pr.Labels[0].Name != "bug" {
		t.Fatalf("PullRequest decoded wrong: %+v", pr)
	}
	if pr.CreatedAt == nil || !pr.CreatedAt.Equal(time.Date(2026, 8, 26, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("created_at not parsed: %v", pr.CreatedAt)
	}

	const issueBody = `{"number": 8, "title": "i", "pull_request": {}}`
	var issue Issue
	if err := json.Unmarshal([]byte(issueBody), &issue); err != nil {
		t.Fatalf("decode Issue: %v", err)
	}
	if issue.PullRequestBody == nil {
		t.Error("Issue with pull_request object: marker not set")
	}

	var plain Issue
	if err := json.Unmarshal([]byte(`{"number": 9}`), &plain); err != nil {
		t.Fatalf("decode plain Issue: %v", err)
	}
	if plain.PullRequestBody != nil {
		t.Error("plain Issue: pull_request marker should stay nil")
	}
}
