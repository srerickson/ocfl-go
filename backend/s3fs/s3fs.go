// s3fs implements backend interfaces for S3 object stores.

package s3fs

import (
	"errors"
	"io/fs"
	"path"
	"sort"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/s3"
	"github.com/aws/aws-sdk-go/service/s3/s3iface"
	"github.com/srerickson/ocfl/backend"
)

var errNotDir = errors.New("not a dir")

type Backend struct {
	cl     s3iface.S3API
	bucket string
}

var _ backend.Interface = (*Backend)(nil)

func New(cl s3iface.S3API, bucket string) *Backend {
	return &Backend{
		cl:     cl,
		bucket: bucket,
	}
}

// Open implements fs.FS.
func (f *Backend) Open(name string) (fs.File, error) {
	// log.Printf("----begin: Open: %s----\n", name)
	//defer log.Printf("----end: Open: %s----\n", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  fs.ErrInvalid,
		}
	}
	if name == "." {
		return openDir(f.cl, f.bucket, name)
	}
	out, err := f.cl.GetObject(&s3.GetObjectInput{
		Key:    &name,
		Bucket: &f.bucket,
	})
	if err != nil {
		if isNotFoundErr(err) {
			switch d, err := openDir(f.cl, f.bucket, name); {
			case err == nil:
				return d, nil
			case !isNotFoundErr(err) && !errors.Is(err, errNotDir) && !errors.Is(err, fs.ErrNotExist):
				return nil, err
			}

			return nil, &fs.PathError{
				Op:   "open",
				Path: name,
				Err:  fs.ErrNotExist,
			}
		}

		return nil, &fs.PathError{
			Op:   "open",
			Path: name,
			Err:  err,
		}
	}
	statFunc := func() (fs.FileInfo, error) {
		return stat(f.cl, f.bucket, name)
	}

	if out.ContentLength != nil && out.LastModified != nil {
		// if we got all the information from GetObjectOutput
		// then we can cache fileinfo instead of making
		// another call in case Stat is called.
		statFunc = func() (fs.FileInfo, error) {
			return &fileInfo{
				name:    path.Base(name),
				size:    *out.ContentLength,
				modTime: *out.LastModified,
			}, nil
		}
	}

	return &file{
		ReadCloser: out.Body,
		stat:       statFunc,
	}, nil
}

func (b *Backend) ReadDir(name string) ([]fs.DirEntry, error) {
	// // log.Printf("----begin: ReadDir: %s----\n", name)
	// defer // log.Printf("----end: ReadDir: %s----\n", name)
	if !fs.ValidPath(name) {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  errors.New("invalid path"),
		}
	}
	var entries []fs.DirEntry
	in := &s3.ListObjectsV2Input{
		Delimiter: aws.String("/"),
		Bucket:    &b.bucket,
	}
	if name != "." {
		in.Prefix = aws.String(name + "/")
	}
	eachPage := func(out *s3.ListObjectsV2Output, last bool) bool {
		for _, c := range out.Contents {
			fi := fileInfo{
				name:    path.Base(*c.Key),
				size:    *c.Size,
				modTime: *c.LastModified,
			}
			entries = append(entries, &dirEntry{fileInfo: fi})
		}
		for _, p := range out.CommonPrefixes {
			fi := fileInfo{
				name: path.Base(*p.Prefix),
				mode: fs.ModeDir,
			}
			entries = append(entries, &dirEntry{fileInfo: fi})
		}
		return last
	}
	// // log.Println("S3=listobjectsv2pages:", name)
	err := b.cl.ListObjectsV2Pages(in, eachPage)
	if err != nil {
		return nil, err
	}
	if len(entries) == 0 {
		return nil, &fs.PathError{
			Op:   "readdir",
			Path: name,
			Err:  fs.ErrNotExist,
		}
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Name() < entries[j].Name()
	})

	return entries, nil
}

// Stat implements fs.StatFS.
func (f *Backend) Stat(name string) (fs.FileInfo, error) {
	fi, err := stat(f.cl, f.bucket, name)
	if err != nil {
		return nil, &fs.PathError{
			Op:   "stat",
			Path: name,
			Err:  err,
		}
	}
	return fi, nil
}

func stat(s3cl s3iface.S3API, bucket, name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	if name == "." {
		return &dir{
			s3cl:   s3cl,
			bucket: bucket,
			fileInfo: fileInfo{
				name: ".",
				mode: fs.ModeDir,
			},
		}, nil
	}
	// default stat is object-first
	return statOrder(s3cl, bucket, name, statObject, statPrefix)
}

type statFunc func(s3iface.S3API, string, string) (fs.FileInfo, error)

// statOrder allows prioritizing statObject or statPrefix, since they require separate S3 requests.
func statOrder(s3cl s3iface.S3API, bucket, name string, a, b statFunc) (fs.FileInfo, error) {
	inf, err := a(s3cl, bucket, name)
	if err == nil {
		return inf, err
	} else if !isNotFoundErr(err) && !errors.Is(err, fs.ErrNotExist) {
		return nil, err
	}
	return b(s3cl, bucket, name)
}

func statObject(s3cl s3iface.S3API, bucket, name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	// log.Println("S3=headobject:", name)
	head, err := s3cl.HeadObject(&s3.HeadObjectInput{
		Bucket: &bucket,
		Key:    aws.String(name),
	})
	if err != nil {
		return nil, err
	}
	return &fileInfo{
		name:    name,
		size:    derefInt64(head.ContentLength),
		mode:    0,
		modTime: derefTime(head.LastModified),
	}, nil
}

func statPrefix(s3cl s3iface.S3API, bucket, name string) (fs.FileInfo, error) {
	if !fs.ValidPath(name) {
		return nil, fs.ErrInvalid
	}
	// log.Println("s3=listobjectsv2, prefix:", name+"/")
	out, err := s3cl.ListObjectsV2(&s3.ListObjectsV2Input{
		Bucket:    &bucket,
		Delimiter: aws.String("/"),
		Prefix:    aws.String(name + "/"),
		MaxKeys:   aws.Int64(1),
	})
	if err != nil {
		return nil, err
	}
	if len(out.CommonPrefixes)+len(out.Contents) == 0 {
		return nil, fs.ErrNotExist
	}
	return &dir{
		s3cl:   s3cl,
		bucket: bucket,
		fileInfo: fileInfo{
			name: name,
			mode: fs.ModeDir,
		},
	}, nil

}

func openDir(s3cl s3iface.S3API, bucket, name string) (fs.ReadDirFile, error) {
	fi, err := statOrder(s3cl, bucket, name, statPrefix, statObject)
	if err != nil {
		return nil, err
	}

	if d, ok := fi.(fs.ReadDirFile); ok {
		return d, nil
	}
	return nil, errNotDir
}

var notFoundCodes = map[string]struct{}{
	s3.ErrCodeNoSuchKey: {},
	"NotFound":          {},
}

func isNotFoundErr(err error) bool {
	if aerr, ok := err.(awserr.Error); ok {
		_, ok := notFoundCodes[aerr.Code()]
		return ok
	}
	return false
}
