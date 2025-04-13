// package http implements and http-based backend that supports basic object
// access.
package http

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"path"
	"time"
)

// file more reported by fs.FileInfo.
const fileMode = 0444 | fs.ModeIrregular

// New returns a new [FS] that resolves files relative to baseURL. It uses
// http.DefaultClient unless you set the http client with [WithClient]().
func New(baseURl string, opts ...Option) *FS {
	fsys := &FS{
		baseURL: baseURl,
	}
	for _, opt := range opts {
		opt(fsys)
	}
	return fsys
}

// FS is an ocfl/fs.FS that reads files over http(s). The remote http server
// must support HEAD and GET requests and response should include Content-Length
// and Last-Modified headers.
type FS struct {
	client  *http.Client
	baseURL string
}

// BaseURL returns the base url used to construct f
func (f FS) BaseURL() string { return f.baseURL }

// Client returns f's http client
func (f FS) Client() *http.Client { return f.client }

// OpenFile implements the ocfl/fs.FS interface for FS
func (f FS) OpenFile(ctx context.Context, name string) (fs.File, error) {
	const op = "openfile"
	if !fs.ValidPath(name) || name == "." {
		return nil, pathError(op, name, fs.ErrInvalid)
	}
	cli := f.client
	if cli == nil {
		cli = http.DefaultClient
	}
	requestURL, err := url.JoinPath(f.baseURL, name)
	if err != nil {
		return nil, pathError(op, name, err)
	}
	rq, err := http.NewRequestWithContext(ctx, http.MethodHead, requestURL, nil)
	if err != nil {
		return nil, pathError(op, name, err)
	}
	resp, err := cli.Do(rq)
	if err != nil {
		return nil, pathError(op, name, err)
	}
	defer func() {
		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	if resp.StatusCode == http.StatusNotFound {
		return nil, pathError(op, name, fs.ErrNotExist)
	}
	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("http: unexpected response status: %q", resp.Status)
		return nil, pathError(op, name, err)
	}
	var modtime time.Time
	if m := resp.Header.Get("Last-Modified"); m != "" {
		modtime, err = http.ParseTime(m)
		if err != nil {
			return nil, fmt.Errorf("parsing last-modified header: %w", err)
		}
	}
	return &httpFile{
		ctx:     ctx,
		client:  cli,
		uri:     requestURL,
		name:    path.Base(name),
		size:    resp.ContentLength,
		modTime: modtime,
	}, nil
}

type httpFile struct {
	ctx     context.Context
	client  *http.Client
	uri     string
	body    io.ReadCloser
	name    string
	size    int64
	modTime time.Time
}

var _ fs.File = (*httpFile)(nil)

func (f *httpFile) Close() error {
	if f.body == nil {
		return nil
	}
	return f.body.Close()
}

func (f *httpFile) Read(b []byte) (int, error) {
	if f.body == nil {
		req, err := http.NewRequestWithContext(f.ctx, http.MethodGet, f.uri, nil)
		if err != nil {
			return 0, err
		}
		resp, err := f.client.Do(req)
		if err != nil {
			return 0, err
		}
		if resp.StatusCode != 200 {
			err = fmt.Errorf("http: unexpected response status: %q", resp.Status)
			return 0, err
		}
		f.body = resp.Body
	}
	return f.body.Read(b)
}

func (f *httpFile) Stat() (fs.FileInfo, error) { return f, nil }
func (f *httpFile) Name() string               { return f.name }
func (f *httpFile) Size() int64                { return f.size }
func (f *httpFile) Mode() fs.FileMode          { return fileMode }
func (f *httpFile) ModTime() time.Time         { return f.modTime }
func (f *httpFile) IsDir() bool                { return false }
func (f *httpFile) Sys() any                   { return nil }

func pathError(op string, name string, err error) error {
	return &fs.PathError{
		Op:   op,
		Path: name,
		Err:  err,
	}
}

// Options are used to configure the FS returned by [New].
type Option func(fsys *FS)

// WithClient is used to set the http.Client used by an FS.
func WithClient(cli *http.Client) Option {
	return func(fsys *FS) {
		fsys.client = cli
	}
}
