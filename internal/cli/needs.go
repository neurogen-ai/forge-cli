package cli

// NeedsAPI is implemented by commands that require a resolved host/owner/repo
// and an authenticated API client. Commands not implementing it (e.g. version,
// cache path) never trigger auth or host validation.
type NeedsAPI interface {
	RequiresAPI() bool
}

// RequiresAPI reports whether c needs network wiring; false by default.
func RequiresAPI(c Command) bool {
	n, ok := c.(NeedsAPI)
	return ok && n.RequiresAPI()
}
