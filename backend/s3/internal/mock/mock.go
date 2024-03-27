package mock

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
	"golang.org/x/exp/rand"
)

var uploadID = "mock-mpu-id"

type S3API struct {
	Bucket  string
	Objects map[string]*Object
	Updated map[string]string
	Parts   sync.Map
}

func (m *S3API) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput, opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	obj, err := m.getObject(in.Key)
	if err != nil {
		return nil, err
	}
	out := &s3v2.HeadObjectOutput{
		ContentLength: aws.Int64(int64(len(obj.Body))),
		LastModified:  aws.Time(obj.LastModified),
	}
	return out, nil
}

func (m *S3API) GetObject(ctx context.Context, in *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	obj, err := m.getObject(in.Key)
	if err != nil {
		return nil, err
	}
	return &s3v2.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewBuffer(obj.Body)),
		ContentLength: aws.Int64(int64(len(obj.Body))),
		LastModified:  aws.Time(obj.LastModified),
	}, nil
}

func (m *S3API) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	maxkeys := 1000
	if in.MaxKeys != nil {
		maxkeys = int(*in.MaxKeys)
	}
	prefix := ""
	if in.Prefix != nil {
		prefix = *in.Prefix
	}
	out := &s3v2.ListObjectsV2Output{
		Name:              in.Bucket,
		Prefix:            in.Prefix,
		Delimiter:         in.Delimiter,
		MaxKeys:           in.MaxKeys,
		ContinuationToken: in.ContinuationToken,
		IsTruncated:       aws.Bool(false),
	}
	for i, key := range m.Keys() {
		if in.ContinuationToken != nil && key <= *in.ContinuationToken {
			continue
		}
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		object := m.Objects[key]
		keySuffix := strings.TrimPrefix(key, prefix)
		// keyPart is first path element after prefix
		keyPart, _, isCommonPrefix := strings.Cut(keySuffix, "/")
		switch {
		case isCommonPrefix:
			// add to common prefixes, if it's not there
			commonPrefix := prefix + "/" + keyPart
			if l := len(out.CommonPrefixes); l > 0 {
				prev := out.CommonPrefixes[l-1]
				if prev.Prefix != nil && *prev.Prefix == commonPrefix {
					break
				}
			}
			out.CommonPrefixes = append(out.CommonPrefixes, types.CommonPrefix{Prefix: &commonPrefix})
		default:
			// add to contents
			cont := types.Object{
				Key:          aws.String(key),
				Size:         aws.Int64(object.ContentLength),
				LastModified: aws.Time(object.LastModified),
			}
			out.Contents = append(out.Contents, cont)
		}
		keyCount := len(out.Contents)
		numCommonPrefixes := len(out.CommonPrefixes)
		if numCommonPrefixes > 0 {
			keyCount += 1
		}
		out.KeyCount = aws.Int32(int32(keyCount))
		haveMoreKeys := i < len(m.Objects)-1
		if haveMoreKeys && keyCount >= int(maxkeys) || numCommonPrefixes >= int(maxkeys) {
			// that's all we can include,
			out.IsTruncated = aws.Bool(true)
			out.NextContinuationToken = aws.String(key)
			break
		}
	}
	return out, nil
}

func (m *S3API) PutObject(ctx context.Context, in *s3v2.PutObjectInput, opts ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("key is nil")
	}
	etag, err := md5(in.Body)
	if err != nil {
		return nil, err
	}
	out := &s3v2.PutObjectOutput{
		ETag: &etag,
	}
	m.Updated[*in.Key] = etag
	return out, nil
}
func (m *S3API) CreateMultipartUpload(ctx context.Context, in *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	out := &s3v2.CreateMultipartUploadOutput{
		Bucket:   in.Bucket,
		Key:      in.Key,
		UploadId: &uploadID,
	}
	return out, nil
}

func (m *S3API) UploadPart(ctx context.Context, in *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("UploaderID is nil")
	}
	if in.PartNumber == nil {
		return nil, errors.New("PartNumber is nil")
	}
	etag, err := md5(in.Body)
	if err != nil {
		return nil, err
	}
	out := &s3v2.UploadPartOutput{
		ETag: &etag,
	}
	m.Parts.Store(*in.PartNumber, etag)
	return out, nil
}
func (m *S3API) UploadPartCopy(ctx context.Context, in *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	out := &s3v2.UploadPartCopyOutput{}
	return out, nil
}
func (m *S3API) CompleteMultipartUpload(ctx context.Context, in *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("invalid uploader id")
	}
	if in.MultipartUpload == nil {
		return nil, errors.New("no uploads")
	}
	etags := make([]string, len(in.MultipartUpload.Parts))
	for _, p := range in.MultipartUpload.Parts {
		if p.PartNumber == nil {
			return nil, errors.New("nil partnumber")
		}
		if p.ETag == nil {
			return nil, errors.New("nil etag in upload parts")
		}
		tag, exists := m.Parts.Load(*p.PartNumber)
		if !exists {
			return nil, fmt.Errorf("unknown part number %d", *p.PartNumber)
		}
		if tag.(string) != *p.ETag {
			return nil, fmt.Errorf("etags don't match for part number %d", *p.PartNumber)
		}
		etags = append(etags, *p.ETag)
	}
	etag, err := md5(strings.NewReader(strings.Join(etags, "")))
	if err != nil {
		return nil, err
	}
	etag += fmt.Sprintf("-%d", len(etags))
	out := &s3v2.CompleteMultipartUploadOutput{
		Bucket: in.Bucket,
		Key:    in.Key,
		ETag:   aws.String(etag),
	}
	m.Updated[*in.Key] = etag
	return out, nil
}

func (m *S3API) AbortMultipartUpload(ctx context.Context, param *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	out := &s3v2.AbortMultipartUploadOutput{}
	return out, nil
}

func (m *S3API) CopyObject(ctx context.Context, param *s3v2.CopyObjectInput, opts ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	out := &s3v2.CopyObjectOutput{}
	return out, nil
}

func (m *S3API) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	if _, ok := m.Objects[*in.Key]; !ok {
		return nil, &types.NoSuchKey{}
	}
	out := &s3v2.DeleteObjectOutput{}
	return out, nil
}

func (m *S3API) Keys() []string {
	keys := make([]string, 0, len(m.Objects))
	for k := range m.Objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m *S3API) bucketOK(b *string) error {
	if !eql(m.Bucket, b) {
		return &types.NoSuchBucket{}
	}
	return nil
}

func (m *S3API) getObject(k *string) (*Object, error) {
	if k == nil {
		return nil, errors.New("object key is nil")
	}
	obj, ok := m.Objects[*k]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return obj, nil
}

var _ s3.S3API = (*S3API)(nil)

type Object struct {
	Body          []byte
	LastModified  time.Time
	ContentLength int64
}

func GenObjects(seed uint64, objCount int, keyPrefix string, depth int, maxFileSize int64) map[string]*Object {
	if depth < 1 {
		depth = 1
	}
	if objCount < 1 {
		objCount = 1
	}
	if maxFileSize < 1 {
		maxFileSize = 1
	}
	gen := rand.New(rand.NewSource(seed))
	objects := make(map[string]*Object, objCount)
	keys := make(map[string]struct{}, objCount)
	dirs := map[string]struct{}{".": {}}
	genKey := func() string {
		for {
			// loop until we get a uniqu key
			var dir string
			switch {
			case depth == 1:
				dir = "."
			case gen.Intn(4) > 0:
				// use an existing directory
				for dir = range dirs {
					break
				}
			default:
				// create a new directory
				dirParts := make([]string, gen.Intn(depth))
				for j := range dirParts {
					dirParts[j] = randPathPart(gen, 2, 8)
				}
				dir = path.Join(dirParts...)
			}
			key := path.Join(dir, randPathPart(gen, 5, 12))
			if _, exists := keys[key]; !exists {
				keys[key] = struct{}{}
				dirs[dir] = struct{}{}
				return key
			}
		}
	}
	for i := 0; i < objCount; i++ {
		key := path.Join(keyPrefix, genKey())
		objects[key] = &Object{
			ContentLength: gen.Int63n(maxFileSize + 1),
			LastModified:  time.Unix(1711391789-int64(gen.Intn(31536000)), 0),
		}
	}
	return objects
}

func randPathPart(genr *rand.Rand, minSize, maxSize int) string {
	const chars = `abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_.`
	const lenChars = len(chars)
	size := minSize
	if size < 1 {
		size = 1
	}
	if maxSize > size {
		size += genr.Intn(maxSize - size + 1)
	}
	out := ""
	for i := 0; i < size; i++ {
		var next byte
		for {
			next = chars[genr.Intn(lenChars)]
			if next == '.' && i > 0 && out[i-1] == '.' {
				// dont allow '..'
				continue // try again
			}
			if size == 1 && next == '.' {
				continue
			}
			break
		}
		out += string(next)
	}
	return out
}

func md5(r io.Reader) (string, error) {
	digester := ocfl.NewDigester(ocfl.MD5)
	if _, err := io.Copy(digester, r); err != nil {
		return "", err
	}
	return digester.String(), nil
}

func eql[T comparable](expect T, ptr *T) bool {
	if ptr == nil {
		return false
	}
	return *ptr == expect
}
