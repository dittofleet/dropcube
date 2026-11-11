package cmd

import (
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dittofleet/dropcube/internal/config"
)

// sendFile sends a file as the body of an authorized request.
func sendFile(client *http.Client, dep *config.Deployment, method, target, path string, want int) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if info.IsDir() {
		return nil, fmt.Errorf("is a directory")
	}

	// A non-nil Body with ContentLength 0 means "unknown length" to net/http,
	// which sends the request chunked. R2 rejects unknown-length streams, so
	// empty files need an explicitly empty body.
	var body io.Reader = &exactReader{r: f, remaining: info.Size()}
	if info.Size() == 0 {
		body = http.NoBody
	}
	req, err := http.NewRequest(method, target, body)
	if err != nil {
		return nil, err
	}
	req.ContentLength = info.Size()
	// The worker defaults absent Content-Type to application/octet-stream;
	// only send a header when there is something better.
	if contentType := detectType(f, path); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	return send(client, dep, req, want)
}

// detectType names a file's type from its extension or, for one the system
// doesn't know, as text if that is what it holds. Which extensions are known
// differs by system (Windows has no .log or .md), and an unknown one would
// otherwise download as bytes instead of opening in the browser. All text
// counts as plain, as a .md often opens with an HTML tag.
func detectType(f io.ReaderAt, path string) string {
	if contentType := mime.TypeByExtension(filepath.Ext(path)); contentType != "" {
		return contentType
	}
	head := make([]byte, 512)
	n, _ := f.ReadAt(head, 0)
	if kind, charset, _ := strings.Cut(http.DetectContentType(head[:n]), ";"); strings.HasPrefix(kind, "text/") {
		return "text/plain;" + charset
	}
	return ""
}

// send makes req with the deployment's token. Anything but the wanted
// status is an error, and otherwise the worker's answer comes back.
func send(client *http.Client, dep *config.Deployment, req *http.Request, want int) ([]byte, error) {
	req.Header.Set("Authorization", "Bearer "+dep.Token)
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body := readBody(resp)
	if resp.StatusCode != want {
		return nil, httpError(resp.StatusCode, body)
	}
	return body, nil
}

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
