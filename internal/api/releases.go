package api

import (
	"fmt"
	"net/url"
	"time"
)

// Release is one repository release with its attached assets. It is a
// subset of the server schema: only the fields forge commands read.
type Release struct {
	ID          int64          `json:"id"`
	TagName     string         `json:"tag_name"`
	Name        string         `json:"name"`
	Body        string         `json:"body"`
	Draft       bool           `json:"draft"`
	Prerelease  bool           `json:"prerelease"`
	PublishedAt *time.Time     `json:"published_at"`
	HTMLURL     string         `json:"html_url"`
	Author      User           `json:"author"`
	Assets      []ReleaseAsset `json:"assets"`
}

// ReleaseAsset is one downloadable file attached to a release.
type ReleaseAsset struct {
	ID                 int64  `json:"id"`
	Name               string `json:"name"`
	Size               int64  `json:"size"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// ListReleases returns every repository release in server order, following
// rel="next" links through the shared paginator.
func (c *Client) ListReleases(owner, repo string) ([]Release, error) {
	return List[Release](c, fmt.Sprintf("/repos/%s/%s/releases", owner, repo), nil)
}

// GetRelease fetches one release by tag; the tag is URL-path escaped.
func (c *Client) GetRelease(owner, repo, tag string) (*Release, error) {
	path := fmt.Sprintf("/repos/%s/%s/releases/%s", owner, repo, url.PathEscape(tag))
	var out Release
	if err := c.Do("GET", path, nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
