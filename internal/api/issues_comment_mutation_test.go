package api

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEditComment(t *testing.T) {
	var gotPath, gotMethod, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.Write([]byte(`{"id":77,"body":"edited","html_url":"https://git.example.com/o/r/issues/4#issuecomment-77"}`))
	}))
	defer ts.Close()

	com, err := NewClient(ts.URL, "tok", 0, nil).EditComment("o", "r", 77, "edited")
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues/comments/77" || gotMethod != "PATCH" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"body":"edited"}` {
		t.Errorf("body = %q", gotBody)
	}
	if com.ID != 77 || com.Body != "edited" {
		t.Errorf("comment = %+v", com)
	}
}

func TestDeleteComment(t *testing.T) {
	var gotPath, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	}))
	defer ts.Close()

	if err := NewClient(ts.URL, "tok", 0, nil).DeleteComment("o", "r", 77); err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues/comments/77" || gotMethod != "DELETE" {
		t.Errorf("request = %s %s", gotMethod, gotPath)
	}
}
