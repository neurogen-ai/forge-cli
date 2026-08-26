package cli

const (
	ExitOK      = 0
	ExitRuntime = 1 // API/runtime error        (PRD §10)
	ExitUsage   = 2 // bad flags / missing value
	ExitContext = 3 // not in repo / no remote
	ExitAuth    = 4 // no token / rejected
	ExitNetwork = 5 // timeout / conn failure
)
