package cmds

import (
	"io"

	"forge/internal/cli"
)

// readDashInput resolves a flag value that may mean stdin. A value of "-"
// reads stdin to EOF; "-" with a nil stdin is a usage error; any other value
// returns unchanged with a nil error.
func readDashInput(value string, stdin io.Reader) ([]byte, error) {
	if value != "-" {
		return []byte(value), nil
	}
	if stdin == nil {
		return nil, &cli.Error{
			Code: cli.ExitUsage,
			Msg:  "stdin: \"-\" given but no input stream is available",
			Hint: "pipe the value in, e.g. \"forge ... --body - < file.txt\", or pass it inline",
		}
	}
	return io.ReadAll(stdin)
}
