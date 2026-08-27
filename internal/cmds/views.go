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
