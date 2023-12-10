// Copyright 2018 The Go Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package modfetch

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	pathpkg "path"
	"strings"
	"sync"
	"time"

	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// A RevInfo describes a single revision in a module repository.
type RevInfo struct {
	Version string    // suggested version string for this revision
	Time    time.Time // commit time

	// These fields are used for Stat of arbitrary rev,
	// but they are not recorded when talking about module versions.
	Name  string `json:"-"` // complete ID in underlying repository
	Short string `json:"-"` // shortened ID, for use in pseudo-version
}

// A Versions describes the available versions in a module repository.
type Versions struct {
	List []string // semver versions
}

// ErrNoCommits is an error equivalent to fs.ErrNotExist indicating that a given
// repository or module contains no commits.
var ErrNoCommits error = noCommitsError{}

type noCommitsError struct{}

func (noCommitsError) Error() string {
	return "no commits"
}
func (noCommitsError) Is(err error) bool {
	return err == fs.ErrNotExist
}

type proxyRepo struct {
	url         *url.URL
	path        string
	redactedURL string

	listLatestOnce sync.Once
	listLatest     *RevInfo
	listLatestErr  error
}

func newProxyRepo(baseURL, path string) (*proxyRepo, error) {
	base, err := url.Parse(baseURL)
	if err != nil {
		return nil, err
	}
	switch base.Scheme {
	case "http", "https":
		// ok
	case "file":
		if *base != (url.URL{Scheme: base.Scheme, Path: base.Path, RawPath: base.RawPath}) {
			return nil, fmt.Errorf("invalid file:// proxy URL with non-path elements: %s", base.Redacted())
		}
	case "":
		return nil, fmt.Errorf("invalid proxy URL missing scheme: %s", base.Redacted())
	default:
		return nil, fmt.Errorf("invalid proxy URL scheme (must be https, http, file): %s", base.Redacted())
	}

	enc, err := module.EscapePath(path)
	if err != nil {
		return nil, err
	}
	redactedURL := base.Redacted()
	base.Path = strings.TrimSuffix(base.Path, "/") + "/" + enc
	base.RawPath = strings.TrimSuffix(base.RawPath, "/") + "/" + pathEscape(enc)
	return &proxyRepo{base, path, redactedURL, sync.Once{}, nil, nil}, nil
}

func (p *proxyRepo) ModulePath() string {
	return p.path
}

// versionError returns err wrapped in a ModuleError for p.path.
func (p *proxyRepo) versionError(version string, err error) error {
	if version != "" && version != module.CanonicalVersion(version) {
		return &module.ModuleError{
			Path: p.path,
			Err: &module.InvalidVersionError{
				Version: version,
				Pseudo:  module.IsPseudoVersion(version),
				Err:     err,
			},
		}
	}

	return &module.ModuleError{
		Path:    p.path,
		Version: version,
		Err:     err,
	}
}

func (p *proxyRepo) getBytes(ctx context.Context, path string) ([]byte, error) {
	body, err := p.getBody(ctx, path)
	if err != nil {
		return nil, err
	}
	defer body.Close()

	b, err := io.ReadAll(body)
	if err != nil {
		// net/http doesn't add context to Body errors, so add it here.
		// (See https://go.dev/issue/52727.)
		return b, &url.Error{Op: "read", URL: strings.TrimSuffix(p.redactedURL, "/") + "/" + path, Err: err}
	}
	return b, nil
}

func (p *proxyRepo) getBody(ctx context.Context, path string) (r io.ReadCloser, err error) {
	fullPath := pathpkg.Join(p.url.Path, path)

	target := *p.url
	target.Path = fullPath
	target.RawPath = pathpkg.Join(target.RawPath, pathEscape(path))

	resp, err := http.Get(target.String())
	if err != nil {
		return nil, err
	}
	return resp.Body, nil
}

func (p *proxyRepo) Versions(ctx context.Context, prefix string) (*Versions, error) {
	data, err := p.getBytes(ctx, "@v/list")
	if err != nil {
		p.listLatestOnce.Do(func() {
			p.listLatest, p.listLatestErr = nil, p.versionError("", err)
		})
		return nil, p.versionError("", err)
	}
	var list []string
	allLine := strings.Split(string(data), "\n")
	for _, line := range allLine {
		f := strings.Fields(line)
		if len(f) >= 1 && semver.IsValid(f[0]) && strings.HasPrefix(f[0], prefix) && !module.IsPseudoVersion(f[0]) {
			list = append(list, f[0])
		}
	}
	p.listLatestOnce.Do(func() {
		p.listLatest, p.listLatestErr = p.latestFromList(ctx, allLine)
	})
	semver.Sort(list)
	return &Versions{List: list}, nil
}

func (p *proxyRepo) latest(ctx context.Context) (*RevInfo, error) {
	p.listLatestOnce.Do(func() {
		data, err := p.getBytes(ctx, "@v/list")
		if err != nil {
			p.listLatestErr = p.versionError("", err)
			return
		}
		list := strings.Split(string(data), "\n")
		p.listLatest, p.listLatestErr = p.latestFromList(ctx, list)
	})
	return p.listLatest, p.listLatestErr
}

func (p *proxyRepo) latestFromList(ctx context.Context, allLine []string) (*RevInfo, error) {
	var (
		bestTime    time.Time
		bestVersion string
	)
	for _, line := range allLine {
		f := strings.Fields(line)
		if len(f) >= 1 && semver.IsValid(f[0]) {
			// If the proxy includes timestamps, prefer the timestamp it reports.
			// Otherwise, derive the timestamp from the pseudo-version.
			var (
				ft time.Time
			)
			if len(f) >= 2 {
				ft, _ = time.Parse(time.RFC3339, f[1])
			} else if module.IsPseudoVersion(f[0]) {
				ft, _ = module.PseudoVersionTime(f[0])
			} else {
				// Repo.Latest promises that this method is only called where there are
				// no tagged versions. Ignore any tagged versions that were added in the
				// meantime.
				continue
			}
			if bestTime.Before(ft) {
				bestTime = ft
				bestVersion = f[0]
			}
		}
	}
	if bestVersion == "" {
		return nil, p.versionError("", ErrNoCommits)
	}

	// Call Stat to get all the other fields, including Origin information.
	return p.Stat(ctx, bestVersion)
}

func (p *proxyRepo) Stat(ctx context.Context, rev string) (*RevInfo, error) {
	encRev, err := module.EscapeVersion(rev)
	if err != nil {
		return nil, p.versionError(rev, err)
	}
	data, err := p.getBytes(ctx, "@v/"+encRev+".info")
	if err != nil {
		return nil, p.versionError(rev, err)
	}
	info := new(RevInfo)
	if err := json.Unmarshal(data, info); err != nil {
		return nil, p.versionError(rev, fmt.Errorf("invalid response from proxy %q: %w", p.redactedURL, err))
	}
	if info.Version != rev && rev == module.CanonicalVersion(rev) && module.Check(p.path, rev) == nil {
		// If we request a correct, appropriate version for the module path, the
		// proxy must return either exactly that version or an error — not some
		// arbitrary other version.
		return nil, p.versionError(rev, fmt.Errorf("proxy returned info for version %s instead of requested version", info.Version))
	}
	return info, nil
}

func (p *proxyRepo) Latest(ctx context.Context) (*RevInfo, error) {
	data, err := p.getBytes(ctx, "@latest")
	if err != nil {
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, p.versionError("", err)
		}
		return p.latest(ctx)
	}
	info := new(RevInfo)
	if err := json.Unmarshal(data, info); err != nil {
		return nil, p.versionError("", fmt.Errorf("invalid response from proxy %q: %w", p.redactedURL, err))
	}
	return info, nil
}

func (p *proxyRepo) GoMod(ctx context.Context, version string) ([]byte, error) {
	if version != module.CanonicalVersion(version) {
		return nil, p.versionError(version, fmt.Errorf("internal error: version passed to GoMod is not canonical"))
	}

	encVer, err := module.EscapeVersion(version)
	if err != nil {
		return nil, p.versionError(version, err)
	}
	data, err := p.getBytes(ctx, "@v/"+encVer+".mod")
	if err != nil {
		return nil, p.versionError(version, err)
	}
	return data, nil
}

const maxZipFile = 500 << 20 // maximum size of downloaded zip file

func (p *proxyRepo) Zip(ctx context.Context, dst io.Writer, version string) error {
	if version != module.CanonicalVersion(version) {
		return p.versionError(version, fmt.Errorf("internal error: version passed to Zip is not canonical"))
	}

	encVer, err := module.EscapeVersion(version)
	if err != nil {
		return p.versionError(version, err)
	}
	path := "@v/" + encVer + ".zip"
	body, err := p.getBody(ctx, path)
	if err != nil {
		return p.versionError(version, err)
	}
	defer body.Close()

	lr := &io.LimitedReader{R: body, N: maxZipFile + 1}
	if _, err := io.Copy(dst, lr); err != nil {
		// net/http doesn't add context to Body errors, so add it here.
		// (See https://go.dev/issue/52727.)
		err = &url.Error{Op: "read", URL: pathpkg.Join(p.redactedURL, path), Err: err}
		return p.versionError(version, err)
	}
	if lr.N <= 0 {
		return p.versionError(version, fmt.Errorf("downloaded zip file too large"))
	}
	return nil
}

// pathEscape escapes s so it can be used in a path.
// That is, it escapes things like ? and # (which really shouldn't appear anyway).
// It does not escape / to %2F: our REST API is designed so that / can be left as is.
func pathEscape(s string) string {
	return strings.ReplaceAll(url.PathEscape(s), "%2F", "/")
}
