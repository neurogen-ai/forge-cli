package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCreateIssueBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Issue{Number: 4})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).CreateIssue("o", "r", CreateIssueInput{Title: "t", Body: "b", Labels: []int64{1, 2}})
	if err != nil {
		t.Fatal(err)
	}
	if gotPath != "/api/v1/repos/o/r/issues" {
		t.Errorf("path = %q", gotPath)
	}
	if iss.Number != 4 {
		t.Errorf("number = %d", iss.Number)
	}
	labels, ok := gotBody["labels"].([]any)
	if !ok || len(labels) != 2 || labels[0] != float64(1) {
		t.Errorf("labels = %v, want [1 2]", gotBody["labels"])
	}
}

func TestAddComment(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		if got := r.Header.Get("Authorization"); got != "token tok" {
			t.Errorf("Authorization = %q", got)
		}
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Comment{ID: 77, HTMLURL: "https://example.test/o/r/issues/6#issuecomment-77"})
	}))
	defer ts.Close()

	com, err := newTestClient(ts).AddComment("o", "r", 6, "nice work")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/issues/6/comments" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"body":"nice work"}` {
		t.Errorf("body = %q", gotBody)
	}
	if com.ID != 77 || com.HTMLURL != "https://example.test/o/r/issues/6#issuecomment-77" {
		t.Errorf("comment = %+v", com)
	}
}

func TestAddCommentAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"issue does not exist"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).AddComment("o", "r", 99, "x")
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("want *APIError, got %T: %v", err, err)
	}
	if apiErr.Status != 404 || apiErr.Message != "issue does not exist" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestGetIssue(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode(Issue{Number: 9})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).GetIssue("o", "r", 9)
	if err != nil || iss.Number != 9 {
		t.Fatalf("%v %+v", err, iss)
	}
	if gotPath != "/api/v1/repos/o/r/issues/9" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestListIssuesSendsTypeIssues(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]Issue{{Number: 1}})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListIssues("o", "r", "open", 1, 5); err != nil {
		t.Fatal(err)
	}
	want := "limit=5&page=1&state=open&type=issues"
	if gotQuery != want {
		t.Errorf("query = %q, want %q", gotQuery, want)
	}
}

func TestListIssuesEmptyStateOmitsParam(t *testing.T) {
	var gotQuery string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode([]Issue{})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).ListIssues("o", "r", "", 1, 10); err != nil {
		t.Fatal(err)
	}
	if gotQuery != "limit=10&page=1&type=issues" {
		t.Errorf("query = %q (state should be omitted)", gotQuery)
	}
}

func TestGetIssueComments(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]Comment{{ID: 1}, {ID: 2}})
	}))
	defer ts.Close()

	cs, err := newTestClient(ts).GetIssueComments("o", "r", 6)
	if err != nil || len(cs) != 2 {
		t.Fatalf("%v %d", err, len(cs))
	}
	if gotPath != "/api/v1/repos/o/r/issues/6/comments" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestListLabels(t *testing.T) {
	var gotPath string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		json.NewEncoder(w).Encode([]Label{{ID: 1, Name: "bug"}, {ID: 2, Name: "docs"}})
	}))
	defer ts.Close()

	ls, err := newTestClient(ts).ListLabels("o", "r")
	if err != nil || len(ls) != 2 || ls[1].Name != "docs" {
		t.Fatalf("%v %+v", err, ls)
	}
	if gotPath != "/api/v1/repos/o/r/labels" {
		t.Errorf("path = %q", gotPath)
	}
}

func TestSetIssueState(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(Issue{Number: 7, State: "closed"})
	}))
	defer ts.Close()

	iss, err := newTestClient(ts).SetIssueState("o", "r", 7, "closed")
	if err != nil || iss.State != "closed" {
		t.Fatalf("%v %+v", err, iss)
	}
	if gotMethod != "PATCH" {
		t.Errorf("method = %q, want PATCH", gotMethod)
	}
	if gotPath != "/api/v1/repos/o/r/issues/7" {
		t.Errorf("path = %q", gotPath)
	}
	if gotBody["state"] != "closed" {
		t.Errorf("body state = %v, want closed", gotBody["state"])
	}

	iss, err = newTestClient(ts).SetIssueState("o", "r", 7, "open")
	if err != nil || gotBody["state"] != "open" {
		t.Fatalf("%v %v body=%v", err, iss, gotBody["state"])
	}
}

func TestEditIssue(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode(Issue{Number: 4, Title: "t", Body: "b"})
	}))
	defer ts.Close()
	c := newTestClient(ts)

	iss, err := c.EditIssue("o", "r", 4, EditIssueInput{Title: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/issues/4" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"title":"t"}` {
		t.Errorf("body = %q", gotBody)
	}
	if iss.Number != 4 || iss.Title != "t" {
		t.Errorf("iss = %+v", iss)
	}

	// Body-only edit omits the empty title field.
	if _, err := c.EditIssue("o", "r", 4, EditIssueInput{Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"body":"b"}` {
		t.Errorf("body-only edit body = %q", gotBody)
	}

	// Both fields together send exactly two keys.
	if _, err := c.EditIssue("o", "r", 4, EditIssueInput{Title: "t", Body: "b"}); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"title":"t","body":"b"}` {
		t.Errorf("full edit body = %q", gotBody)
	}

	// SetIssueState still rides patchIssue and sends the same state body.
	if _, err := c.SetIssueState("o", "r", 4, "closed"); err != nil {
		t.Fatal(err)
	}
	if gotBody != `{"state":"closed"}` {
		t.Errorf("SetIssueState body after refactor = %q", gotBody)
	}
}

func TestEditIssueError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(422)
		io.WriteString(w, `{"message":"validation failed"}`)
	}))
	defer ts.Close()

	_, err := newTestClient(ts).EditIssue("o", "r", 4, EditIssueInput{Title: "t"})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.Status != 422 || apiErr.Message != "validation failed" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestAddLabels(t *testing.T) {
	var gotMethod, gotPath, gotBody string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		w.WriteHeader(200)
		json.NewEncoder(w).Encode([]Label{{ID: 3, Name: "bug", Color: "ee0701"}, {ID: 7, Name: "docs", Color: "c5def5"}})
	}))
	defer ts.Close()

	labels, err := newTestClient(ts).AddLabels("o", "r", 5, []int64{3, 7})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/issues/5/labels" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody != `{"labels":[3,7]}` {
		t.Errorf("body = %q", gotBody)
	}
	if len(labels) != 2 || labels[0].ID != 3 || labels[0].Name != "bug" || labels[1].ID != 7 {
		t.Errorf("labels = %+v", labels)
	}
}

func TestAddLabelsAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"issue does not exist"}`))
	}))
	defer ts.Close()

	_, err := newTestClient(ts).AddLabels("o", "r", 9, []int64{3})
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.Status != 404 || apiErr.Message != "issue does not exist" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestRemoveLabel(t *testing.T) {
	var gotMethod, gotPath string
	var gotBody []byte
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(204)
	}))
	defer ts.Close()

	if err := newTestClient(ts).RemoveLabel("o", "r", 5, 3); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/v1/repos/o/r/issues/5/labels/3" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if len(gotBody) != 0 {
		t.Errorf("body = %q, want empty", gotBody)
	}
}

func TestRemoveLabelAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"label does not exist"}`))
	}))
	defer ts.Close()

	err := newTestClient(ts).RemoveLabel("o", "r", 5, 3)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.Status != 404 || apiErr.Message != "label does not exist" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func TestCreateLabel(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.WriteHeader(201)
		json.NewEncoder(w).Encode(Label{ID: 9, Name: "triage", Color: "#00aabb"})
	}))
	defer ts.Close()

	lbl, err := newTestClient(ts).CreateLabel("o", "r", "triage", "#00aabb", "needs triage first")
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "POST" || gotPath != "/api/v1/repos/o/r/labels" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "triage" || gotBody["color"] != "#00aabb" || gotBody["description"] != "needs triage first" {
		t.Errorf("body = %v", gotBody)
	}
	if lbl.ID != 9 || lbl.Name != "triage" || lbl.Color != "#00aabb" {
		t.Errorf("label = %+v", lbl)
	}
}

func TestEditLabel(t *testing.T) {
	var gotPath, gotMethod string
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(Label{ID: 9, Name: "renamed", Color: "#ff0000"})
	}))
	defer ts.Close()

	lbl, err := newTestClient(ts).EditLabel("o", "r", 9, UpdateLabelInput{Name: "renamed", Color: "#ff0000"})
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != "PATCH" || gotPath != "/api/v1/repos/o/r/labels/9" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
	if gotBody["name"] != "renamed" || gotBody["color"] != "#ff0000" {
		t.Errorf("body = %v", gotBody)
	}
	if lbl.Name != "renamed" {
		t.Errorf("label = %+v", lbl)
	}
}

func TestEditLabelOmitsZeroFields(t *testing.T) {
	var gotBody map[string]any
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewDecoder(r.Body).Decode(&gotBody)
		json.NewEncoder(w).Encode(Label{ID: 9})
	}))
	defer ts.Close()

	if _, err := newTestClient(ts).EditLabel("o", "r", 9, UpdateLabelInput{Color: "#123456"}); err != nil {
		t.Fatal(err)
	}
	if _, ok := gotBody["name"]; ok {
		t.Errorf("name sent for zero field: %v", gotBody)
	}
	if gotBody["color"] != "#123456" {
		t.Errorf("body = %v", gotBody)
	}
}

func TestDeleteLabel(t *testing.T) {
	var gotPath, gotMethod string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(204)
	}))
	defer ts.Close()

	if err := newTestClient(ts).DeleteLabel("o", "r", 9); err != nil {
		t.Fatal(err)
	}
	if gotMethod != "DELETE" || gotPath != "/api/v1/repos/o/r/labels/9" {
		t.Errorf("got %s %s", gotMethod, gotPath)
	}
}

func TestDeleteLabelAPIError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte(`{"message":"label does not exist"}`))
	}))
	defer ts.Close()

	err := newTestClient(ts).DeleteLabel("o", "r", 9)
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("err = %T, want *APIError", err)
	}
	if apiErr.Status != 404 || apiErr.Message != "label does not exist" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}
