package cmd

import (
	"fmt"
	"io"
	"net/http"
	"strings"
)

// readBody reads a worker response, capped: every response the CLI cares
// about is one link or one short error line. A read failure yields whatever
// arrived, since the status code is the part that decides the outcome.
func readBody(resp *http.Response) []byte {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return body
}

// httpError describes an unexpected response from the worker, folding in
// the short message the worker sends with each of its failures.
func httpError(status int, body []byte) error {
	return fmt.Errorf("HTTP %d: %s", status, strings.TrimSpace(string(body)))
}
