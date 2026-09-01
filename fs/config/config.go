// Package config builds ocfl-go storage backends from URL-style configuration
// strings, and renders them back into those strings.
//
// A configuration string names a backend with its scheme and carries whatever
// that backend needs to be constructed:
//
//	file:///srv/ocfl                        a local directory
//	/srv/ocfl                               the same, without the scheme
//	s3://bucket/prefix?region=us-west-2     an s3 bucket, with "prefix" as the path
//	https://example.org/ocfl                files read over http
//
// The FS is rooted as deeply as the backend allows and [FSConfig.Path] carries
// what is left over. Only s3 has a leftover, because a bucket FS is scoped to
// the whole bucket: for the other backends the path becomes part of the FS
// itself and Path is ".".
//
// An s3 string builds its client from the default AWS configuration, with the
// region, endpoint and path-style query parameters applied on top. Clients are
// reused: strings with the same s3 settings share one, which is what lets
// [ocflfs.Copy] see two bucket file systems as one backend and copy within the
// bucket instead of moving every byte through this process. A file system
// holding a client of your own -- from [WithS3Client] or [s3.NewBucketFS] --
// shares a backend only with file systems holding that same client. The
// ambient AWS configuration is read once per distinct set of settings, so
// later changes to the environment are not picked up. Credentials are not part
// of those settings: a shared client keeps the credential provider it was
// built with, which refreshes on its own for a role or a profile but not when
// the process's own environment variables change. [ResetS3Clients] drops the
// clients so that later strings build new ones.
package config

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
	"sync"

	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	awss3 "github.com/aws/aws-sdk-go-v2/service/s3"
	ocflfs "github.com/srerickson/ocfl-go/fs"
	ocflhttp "github.com/srerickson/ocfl-go/fs/http"
	"github.com/srerickson/ocfl-go/fs/local"
	"github.com/srerickson/ocfl-go/fs/s3"
)

// FSConfig is an [ocflfs.FS] together with a path inside it. It is built from
// a configuration string with [New] and rendered back to one with
// [FSConfig.MarshalText].
//
// An FSConfig is not itself an [ocflfs.FS]: pass the FS field to functions
// that want one. The file system package decides what a backend can do by
// asking the value it is given -- [ocflfs.ReadDir] for [ocflfs.DirEntriesFS],
// [ocflfs.Copy] for [ocflfs.CopyFS] -- and a wrapper only answers for the
// methods it carries. Handing on the FS itself keeps those questions
// answerable.
type FSConfig struct {
	// FS is the storage backend the configuration string named.
	FS ocflfs.FS

	// Path is the directory within FS that the configuration string referred
	// to. It is "." when the string named no subdirectory.
	Path string
}

// Option is a configuration option for [New].
type Option func(*options)

type options struct {
	s3Client   s3.S3API
	httpClient *http.Client
	logger     *slog.Logger
}

// WithS3Client sets the client used for an "s3" configuration string. It is
// used as-is, so no AWS configuration is loaded, the region, endpoint and
// path-style query parameters are ignored, and the client [New] would
// otherwise share is neither built nor used.
func WithS3Client(cli s3.S3API) Option {
	return func(o *options) { o.s3Client = cli }
}

// WithHTTPClient sets the client used for an "http" or "https" configuration
// string. It is ignored for other backends.
func WithHTTPClient(cli *http.Client) Option {
	return func(o *options) { o.httpClient = cli }
}

// WithLogger sets a logger used for debug-level messages by backends that
// support one. Only the s3 backend does.
func WithLogger(logger *slog.Logger) Option {
	return func(o *options) { o.logger = logger }
}

// New returns the [FSConfig] described by conf. It returns an error if conf
// cannot be parsed, if its scheme names an unknown backend, or if the backend
// cannot be constructed -- a local directory that does not exist, for example.
func New(ctx context.Context, conf string, opts ...Option) (*FSConfig, error) {
	getopts := &options{}
	for _, opt := range opts {
		if opt != nil {
			opt(getopts)
		}
	}
	if conf == "" {
		return nil, errors.New("empty fs configuration string")
	}
	u, err := url.Parse(conf)
	if err != nil {
		return nil, fmt.Errorf("parsing fs configuration: %w", err)
	}
	// A one-letter scheme is a windows drive letter, not a backend: "C:\dir"
	// parses as the scheme "c". Treat it, and anything with no scheme at all,
	// as a path on the local file system.
	if len(u.Scheme) < 2 {
		return newLocal(conf)
	}
	switch u.Scheme {
	case "file":
		return newFile(u)
	case "s3":
		return newS3(ctx, u, getopts)
	case "http", "https":
		return newHTTP(conf, u, getopts)
	}
	return nil, fmt.Errorf("fs configuration %q: unsupported scheme %q", u.Redacted(), u.Scheme)
}

// newLocal returns an FSConfig for a path on the local file system.
func newLocal(name string) (*FSConfig, error) {
	fsys, err := local.NewFS(name)
	if err != nil {
		return nil, fmt.Errorf("fs configuration %q: %w", name, err)
	}
	return &FSConfig{FS: fsys, Path: "."}, nil
}

// newFile returns an FSConfig for a file:// url. The url's path is the storage
// root, so the returned Path is always ".".
func newFile(u *url.URL) (*FSConfig, error) {
	if u.User != nil {
		return nil, fmt.Errorf("fs configuration %q: a file url takes no user information", u.Redacted())
	}
	if u.Host != "" && u.Host != "localhost" {
		return nil, fmt.Errorf("fs configuration %q: a file url's host must be empty or 'localhost'", u.Redacted())
	}
	if u.RawQuery != "" {
		return nil, fmt.Errorf("fs configuration %q: a file url takes no query parameters", u.Redacted())
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("fs configuration %q: a file url takes no fragment", u.Redacted())
	}
	if u.Opaque != "" || u.Path == "" {
		return nil, fmt.Errorf("fs configuration %q: a file url must have an absolute path, as in 'file:///srv/ocfl'", u.Redacted())
	}
	return newLocal(urlPathToFilePath(u.Path))
}

// urlPathToFilePath converts the path component of a file url to a path in the
// local file system's syntax.
func urlPathToFilePath(p string) string {
	// windows: the path component of "file:///C:/dir" is "/C:/dir"; the
	// leading separator is not part of the path.
	if len(p) > 2 && p[0] == '/' && p[2] == ':' {
		p = p[1:]
	}
	return filepath.FromSlash(p)
}

// newS3 returns an FSConfig for an s3:// url. The url's host is the bucket and
// its path is the returned Path, since the bucket FS spans the whole bucket.
func newS3(ctx context.Context, u *url.URL, getopts *options) (*FSConfig, error) {
	// credentials belong to the AWS configuration, not to the url: an s3
	// string that carries them would have them silently ignored, and would put
	// a secret in every error message that quoted it.
	if u.User != nil {
		return nil, fmt.Errorf("fs configuration %q: an s3 url takes no user information; credentials come from the AWS configuration", u.Redacted())
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("fs configuration %q: an s3 url takes no fragment", u.Redacted())
	}
	bucket := u.Host
	if bucket == "" {
		return nil, fmt.Errorf("fs configuration %q: missing bucket name, as in 's3://bucket'", u.Redacted())
	}
	if u.Port() != "" {
		return nil, fmt.Errorf("fs configuration %q: %q is not a bucket name; an alternate host goes in the 'endpoint' query parameter", u.Redacted(), bucket)
	}
	dir := strings.Trim(u.Path, "/")
	if dir == "" {
		dir = "."
	}
	if !fs.ValidPath(dir) {
		return nil, fmt.Errorf("fs configuration %q: invalid path %q", u.Redacted(), dir)
	}
	settings, err := parseS3Query(u.Query())
	if err != nil {
		return nil, fmt.Errorf("fs configuration %q: %w", u.Redacted(), err)
	}
	client := getopts.s3Client
	if client == nil {
		if client, err = s3Client(ctx, settings); err != nil {
			return nil, fmt.Errorf("fs configuration %q: %w", u.Redacted(), err)
		}
	}
	var bucketOpts []func(*s3.BucketFS)
	if getopts.logger != nil {
		bucketOpts = append(bucketOpts, s3.WithLogger(getopts.logger))
	}
	return &FSConfig{FS: s3.NewBucketFS(client, bucket, bucketOpts...), Path: dir}, nil
}

// s3Settings holds the parts of an s3 client's configuration that an s3:// url
// can carry.
type s3Settings struct {
	region    string
	endpoint  string
	pathStyle bool
}

// parseS3Query reads the query parameters of an s3:// url. Unrecognized
// parameters are an error, so that a misspelled one is not silently ignored.
func parseS3Query(query url.Values) (s3Settings, error) {
	settings := s3Settings{
		region:   query.Get("region"),
		endpoint: query.Get("endpoint"),
	}
	for key := range query {
		switch key {
		case "region", "endpoint", "path-style":
		default:
			return settings, fmt.Errorf("unknown query parameter %q: expected 'region', 'endpoint' or 'path-style'", key)
		}
	}
	switch value := query.Get("path-style"); value {
	case "", "false", "0":
	case "true", "1":
		settings.pathStyle = true
	default:
		return settings, fmt.Errorf("invalid value for 'path-style': %q", value)
	}
	return settings, nil
}

// s3Clients holds the clients [New] has built, keyed by the settings that
// produced them.
var s3Clients struct {
	sync.Mutex
	clients map[s3Settings]*awss3.Client
}

// s3Client returns the client for settings, building it the first time it is
// asked for and reusing it after that.
//
// The reuse is deliberate. [s3.BucketFS] compares clients by identity to
// decide whether two file systems share a backend, so a client built fresh
// for every call would make two bucket file systems built from equivalent
// configuration strings look like different backends -- and a copy between
// them would move every byte through this process instead of using the
// bucket's own copy operation.
//
// A client is cached under the settings that asked for it and under the
// settings it resolved to, which are not the same when the configuration
// string leaves a region or an endpoint to the environment. That is what
// makes a string like "s3://bucket" share its client with the fully
// spelled-out string it marshals to.
func s3Client(ctx context.Context, settings s3Settings) (*awss3.Client, error) {
	if cli := cachedS3Client(settings); cli != nil {
		return cli, nil
	}
	cli, err := newS3Client(ctx, settings)
	if err != nil {
		return nil, err
	}
	resolved := resolvedS3Settings(cli)
	s3Clients.Lock()
	defer s3Clients.Unlock()
	// a client for either key may have appeared while this one was loading
	// aws configuration. Keep whichever was already handed out, so that every
	// file system with these settings shares a single client.
	if existing := s3Clients.clients[settings]; existing != nil {
		cli = existing
	} else if existing := s3Clients.clients[resolved]; existing != nil {
		cli = existing
	}
	if s3Clients.clients == nil {
		s3Clients.clients = map[s3Settings]*awss3.Client{}
	}
	s3Clients.clients[settings] = cli
	s3Clients.clients[resolved] = cli
	return cli, nil
}

// ResetS3Clients drops the s3 clients [New] has built, so that later
// configuration strings build new ones. Use it when the process's AWS
// configuration has changed -- new credentials in the environment, a different
// profile -- since the clients are otherwise kept for the life of the process.
//
// File systems already built keep the clients they hold, so one built before a
// reset and one built after do not share a backend.
func ResetS3Clients() {
	s3Clients.Lock()
	defer s3Clients.Unlock()
	s3Clients.clients = nil
}

// cachedS3Client returns the client cached for settings, or nil.
func cachedS3Client(settings s3Settings) *awss3.Client {
	s3Clients.Lock()
	defer s3Clients.Unlock()
	return s3Clients.clients[settings]
}

// resolvedS3Settings returns the settings cli ended up with, which include
// whatever the ambient AWS configuration supplied.
func resolvedS3Settings(cli *awss3.Client) s3Settings {
	opts := cli.Options()
	settings := s3Settings{region: opts.Region, pathStyle: opts.UsePathStyle}
	if opts.BaseEndpoint != nil {
		settings.endpoint = *opts.BaseEndpoint
	}
	return settings
}

// newS3Client builds an s3 client from the default AWS configuration, with
// settings applied on top of it.
func newS3Client(ctx context.Context, settings s3Settings) (*awss3.Client, error) {
	var awsOpts []func(*awsconfig.LoadOptions) error
	if settings.region != "" {
		awsOpts = append(awsOpts, awsconfig.WithRegion(settings.region))
	}
	cnf, err := awsconfig.LoadDefaultConfig(ctx, awsOpts...)
	if err != nil {
		return nil, err
	}
	return awss3.NewFromConfig(cnf, func(o *awss3.Options) {
		if settings.endpoint != "" {
			o.BaseEndpoint = &settings.endpoint
		}
		o.UsePathStyle = settings.pathStyle
	}), nil
}

// newHTTP returns an FSConfig for an http:// or https:// url. The whole url is
// the FS's base url, so the returned Path is always ".".
func newHTTP(conf string, u *url.URL, getopts *options) (*FSConfig, error) {
	if u.RawQuery != "" {
		return nil, fmt.Errorf("fs configuration %q: an http url takes no query parameters", u.Redacted())
	}
	if u.Fragment != "" {
		return nil, fmt.Errorf("fs configuration %q: an http url takes no fragment", u.Redacted())
	}
	var httpOpts []ocflhttp.Option
	if getopts.httpClient != nil {
		httpOpts = append(httpOpts, ocflhttp.WithClient(getopts.httpClient))
	}
	return &FSConfig{FS: ocflhttp.New(conf, httpOpts...), Path: "."}, nil
}
