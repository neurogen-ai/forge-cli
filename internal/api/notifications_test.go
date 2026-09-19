package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListNotificationsDefaultUnreadOnly(t *testing.T) {
	var gotPath, gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotQuery = r.URL.Path, r.URL.RawQuery
		json.NewEncoder(w).Encode([]Notification{{ID: 3, Unread: true}})
	}))
	defer ts.Close()

	ns, err := newTestClient(ts).ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/notifications" {
		t.Errorf("path = %q", gotPath)
	}
	if gotQuery != "" {
		t.Errorf("query = %q, want no all param by default", gotQuery)
	}
	if len(ns) != 1 || ns[0].ID != 3 || !ns[0].Unread {
		t.Errorf("ns = %+v", ns)
	}
}

func TestListNotificationsAllParam(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]Notification{})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListNotifications(true); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "all=true" {
		t.Errorf("query = %q, want all=true", gotQuery)
	}
}

func TestListNotificationsDecodesSubjectAndRepo(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`[{"id":7,"unread":true,"subject":{"title":"re: fix","type":"PullRequest","url":"http://x/1"},"repository":{"full_name":"o/r"}}]`))
	}))
	defer ts.Close()

	ns, err := newTestClient(ts).ListNotifications(false)
	if err != nil {
		t.Fatal(err)
	}
	if len(ns) != 1 {
		t.Fatalf("len(ns) = %d", len(ns))
	}
	n := ns[0]
	if n.Subject.Title != "re: fix" || n.Subject.Type != "PullRequest" || n.Repository.FullName != "o/r" {
		t.Errorf("n = %+v", n)
	}
}

func TestMarkNotificationsReadSendsBatchIDs(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(205)
	}))
	defer ts.Close()

	if err := newTestClient(ts).MarkNotificationsRead([]int64{4, 9}); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PUT" || gotPath != "/api/v1/notifications" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	ids, _ := gotBody["ids"].([]any)
	if len(ids) != 2 || ids[0] != float64(4) || ids[1] != float64(9) {
		t.Errorf("body = %v", gotBody)
	}
}

func TestMarkNotificationsReadFallsBackToSequentialThreads(t *testing.T) {
	var paths []string
	var methods []string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/api/v1/notifications" {
			w.WriteHeader(404)
			return
		}
		w.WriteHeader(205)
	}))
	defer ts.Close()

	if err := newTestClient(ts).MarkNotificationsRead([]int64{4, 9}); err != nil {
		t.Fatal(err)
	}
	want := []string{"/api/v1/notifications", "/api/v1/notifications/threads/4", "/api/v1/notifications/threads/9"}
	if len(paths) != 3 {
		t.Fatalf("paths = %v", paths)
	}
	for i := range want {
		if paths[i] != want[i] || methods[i] != "PUT" {
			t.Errorf("request %d = %s %s, want PUT %s", i, methods[i], paths[i], want[i])
		}
	}
}

func TestMarkNotificationsReadEmptyIsNoRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("unexpected request for empty ids")
	}))
	defer ts.Close()

	if err := newTestClient(ts).MarkNotificationsRead(nil); err != nil {
		t.Fatal(err)
	}
}
