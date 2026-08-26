package api

import "fmt"

// APIError is returned by Client.Do for every non-2xx response. Status is the
// HTTP status code; Message comes from the server body's "message" field,
// falling back to the status text.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%d: %s", e.Status, e.Message)
}
