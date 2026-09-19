package api

import (
	"encoding/json"
	"testing"
)

func TestNotificationDecode(t *testing.T) {
	raw := `{
		"id": 42,
		"unread": true,
		"subject": {"title":"fix the leak","type":"PullRequest","url":"https://git.test/o/r/pulls/7"},
		"repository": {"id": 3, "full_name": "o/r", "owner": {"id": 1, "login": "o"}}
	}`
	var got Notification
	if err := json.Unmarshal([]byte(raw), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != 42 || !got.Unread {
		t.Errorf("id/unread = %d %v", got.ID, got.Unread)
	}
	if got.Subject.Title != "fix the leak" || got.Subject.Type != "PullRequest" ||
		got.Subject.URL != "https://git.test/o/r/pulls/7" {
		t.Errorf("subject = %+v", got.Subject)
	}
	if got.Repository.FullName != "o/r" || got.Repository.Owner.Login != "o" {
		t.Errorf("repository = %+v", got.Repository)
	}
}
