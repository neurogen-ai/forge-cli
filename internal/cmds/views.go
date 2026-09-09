package cmds

import (
	"strconv"
	"time"

	"forge/internal/api"
	"forge/internal/table"
)

var prListColumns = []table.Column{
	{Name: "NUMBER", Width: 8},
	{Name: "TITLE", Width: 44},
	{Name: "STATE", Width: 7},
	{Name: "USER", Width: 14},
	{Name: "UPDATED", Width: 10},
}

var issueListColumns = []table.Column{
	{Name: "NUMBER", Width: 8},
	{Name: "TITLE", Width: 48},
	{Name: "STATE", Width: 7},
	{Name: "USER", Width: 16},
}

func timeShort(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func prListRows(prs []api.PullRequest) [][]string {
	rows := make([][]string, 0, len(prs))
	for _, p := range prs {
		rows = append(rows, []string{
			strconv.FormatInt(p.Number, 10),
			p.Title,
			p.State,
			p.User.Login,
			timeShort(p.CreatedAt),
		})
	}
	return rows
}

var reviewListColumns = []table.Column{
	{Name: "REVIEW", Width: 8},
	{Name: "USER", Width: 14},
	{Name: "STATE", Width: 15},
	{Name: "UNRESOLVED", Width: 10},
	{Name: "TOTAL", Width: 7},
}

// issueListRows mirrors prListRows over api.Issue; UPDATED column omitted there by spec.
func issueListRows(iss []api.Issue) [][]string {
	rows := make([][]string, 0, len(iss))
	for _, i := range iss {
		rows = append(rows, []string{
			strconv.FormatInt(i.Number, 10),
			i.Title,
			i.State,
			i.User.Login,
		})
	}
	return rows
}

var labelListColumns = []table.Column{
	{Name: "NAME", Width: 24},
	{Name: "COLOR", Width: 8},
	{Name: "ID", Width: 10},
}

func labelListRows(labels []api.Label) [][]string {
	rows := make([][]string, 0, len(labels))
	for _, l := range labels {
		rows = append(rows, []string{
			l.Name,
			l.Color,
			strconv.FormatInt(l.ID, 10),
		})
	}
	return rows
}

var releaseListColumns = []table.Column{
	{Name: "TAG", Width: 14},
	{Name: "NAME", Width: 36},
	{Name: "DRAFT", Width: 5},
	{Name: "PRERELEASE", Width: 10},
	{Name: "PUBLISHED", Width: 10},
}

func releaseListRows(rels []api.Release) [][]string {
	rows := make([][]string, 0, len(rels))
	for _, r := range rels {
		rows = append(rows, []string{
			r.TagName,
			r.Name,
			strconv.FormatBool(r.Draft),
			strconv.FormatBool(r.Prerelease),
			timeShort(r.PublishedAt),
		})
	}
	return rows
}
