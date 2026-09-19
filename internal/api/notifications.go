package api

// Notification models one entry of the notifications list. Read state stays
// server-side; nothing in this package fetches or stores more than the list
// and read endpoints return.
type Notification struct {
	ID     int64 `json:"id"`
	Unread bool  `json:"unread"`

	Subject struct {
		Title string `json:"title"`
		Type  string `json:"type"` // Issue|PullRequest|Commit|Release
		URL   string `json:"url"`
	} `json:"subject"`

	Repository Repository `json:"repository"`
}
