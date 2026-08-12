package cmd

import (
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sylophi/dropcube/internal/config"
)

const uploadUsage = "usage: dropcube upload <file>..."

const (
	uploadTimeout     = 10 * time.Minute
	uploadConcurrency = 4
)

// Upload sends each file to the configured worker endpoint and prints one
// view link per file, in argument order, to stdout.
func Upload(args []string) error {
	if err := rejectUnknownFlags(args, uploadUsage); err != nil {
		return err
	}
	if len(args) == 0 {
		return fmt.Errorf("no files given\n%s", uploadUsage)
	}

	cfg, err := config.Load()
	if err != nil {
		return err
	}

	client := &http.Client{Timeout: uploadTimeout}
	links := make([]string, len(args))
	errs := make([]error, len(args))
	sem := make(chan struct{}, uploadConcurrency)
	var wg sync.WaitGroup
	for i, path := range args {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			links[i], errs[i] = uploadFile(client, cfg, path)
		}()
	}
	wg.Wait()

	var failures []error
	for i, path := range args {
		if errs[i] != nil {
			failures = append(failures, fmt.Errorf("upload %s: %w", path, errs[i]))
			continue
		}
		fmt.Println(links[i])
	}
	return errors.Join(failures...)
}

func uploadFile(client *http.Client, cfg *config.Config, path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return "", err
	}
	if info.IsDir() {
		return "", fmt.Errorf("is a directory")
	}

	name := filepath.Base(path)
	// Set Path (unescaped) and clear RawPath so URL.String escapes the
	// filename for us. JoinPath would instead read it as already-escaped and
	// mangle any name containing a percent sign.
	target := *cfg.URL
	target.Path = strings.TrimSuffix(cfg.URL.Path, "/") + "/" + name
	target.RawPath = ""

	// A non-nil Body with ContentLength 0 means "unknown length" to net/http,
	// which sends the request chunked. R2 rejects unknown-length streams, so
	// empty files need an explicitly empty body.
	var reqBody io.Reader = &exactReader{r: f, remaining: info.Size()}
	if info.Size() == 0 {
		reqBody = http.NoBody
	}
	req, err := http.NewRequest(http.MethodPut, target.String(), reqBody)
	if err != nil {
		return "", err
	}
	req.ContentLength = info.Size()
	req.Header.Set("Authorization", "Bearer "+cfg.Token)
	// The worker defaults absent Content-Type to application/octet-stream;
	// only send a header when the extension yields something better.
	if contentType := mime.TypeByExtension(filepath.Ext(name)); contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	link := strings.TrimSpace(string(body))
	if u, err := url.Parse(link); err != nil || !u.IsAbs() || u.Host == "" {
		return "", fmt.Errorf("unexpected response: %s", link)
	}
	return link, nil
}

// exactReader yields exactly the byte count that was promised as the
// request's Content-Length. A file being appended to (a live build log, say)
// would otherwise overrun that count and fail the transfer with a transport
// error after the worker had already stored the object.
type exactReader struct {
	r         io.Reader
	remaining int64
}

func (e *exactReader) Read(p []byte) (int, error) {
	if e.remaining <= 0 {
		return 0, io.EOF
	}
	if int64(len(p)) > e.remaining {
		p = p[:e.remaining]
	}
	n, err := e.r.Read(p)
	e.remaining -= int64(n)
	if err == io.EOF && e.remaining > 0 {
		return n, fmt.Errorf("file shrank while uploading (%d bytes short)", e.remaining)
	}
	return n, err
}
