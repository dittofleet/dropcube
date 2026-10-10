package cmd

import (
	"cmp"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/dittofleet/dropcube/internal/config"
)

const uploadUsage = "usage: dropcube upload [--to <deployment>] <file>..."

const (
	uploadTimeout     = 10 * time.Minute
	uploadConcurrency = 4
)

// Upload sends each file to the default deployment, or the one named with
// --to, and prints one view link per file, in argument order, to stdout.
func Upload(args []string) error {
	args, to, err := extractValueFlag(args, "to")
	if err != nil {
		return fmt.Errorf("%w\n%s", err, uploadUsage)
	}
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
	target, err := cfg.Find(cmp.Or(to, config.DefaultName))
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
			links[i], errs[i] = uploadFile(client, target, path)
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

func uploadFile(client *http.Client, dep *config.Deployment, path string) (string, error) {
	// Set Path (unescaped) and clear RawPath so URL.String escapes the
	// filename for us. JoinPath would instead read it as already-escaped and
	// mangle any name containing a percent sign.
	target := *dep.URL
	target.Path = strings.TrimSuffix(dep.URL.Path, "/") + "/" + filepath.Base(path)
	target.RawPath = ""

	body, err := sendFile(client, dep, http.MethodPut, target.String(), path, http.StatusCreated)
	if err != nil {
		return "", err
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
