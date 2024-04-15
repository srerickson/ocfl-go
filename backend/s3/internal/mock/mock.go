package mock

import (
	"bytes"
	"context"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/srerickson/ocfl-go/backend/s3"
)

var (
	uploadID    = "mock-mpu-id"
	byteRangeRE = regexp.MustCompile(`^bytes=\d+-\d+$`)
)

func New(bucket string, objects ...*Object) *S3API {
	api := &S3API{
		bucket:       bucket,
		objects:      make(map[string]*Object, len(objects)),
		UpdatedETags: map[string]string{},
		Deleted:      map[string]bool{},
	}
	for _, b := range objects {
		api.objects[b.Key] = b
	}
	return api
}

type S3API struct {
	UpdatedETags map[string]string
	Deleted      map[string]bool
	MPUCreated   bool
	MPUAborted   bool
	MPUComplete  bool

	CopyObjectFunc func(context.Context, *s3v2.CopyObjectInput, ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error)

	parts   sync.Map
	bucket  string
	objects map[string]*Object
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
	for i, key := range m.objectKeys() {
		if in.ContinuationToken != nil && key <= *in.ContinuationToken {
			continue
		}
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		object := m.objects[key]
		keySuffix := strings.TrimPrefix(key, prefix)
		var suffixFirstPart string
		var isCommonPrefix bool
		if in.Delimiter != nil {
			// suffixFirstPart is first path element in suffix
			suffixFirstPart, _, isCommonPrefix = strings.Cut(keySuffix, *in.Delimiter)
		}
		switch {
		case isCommonPrefix:
			// add to common prefixes, if it's not there
			commonPrefix := prefix + *in.Delimiter + suffixFirstPart
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
		haveMoreKeys := i < len(m.objects)-1
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
		return nil, errors.New("key is required")
	}
	etag, err := md5hex(in.Body)
	if err != nil {
		return nil, err
	}
	out := &s3v2.PutObjectOutput{
		ETag: &etag,
	}
	m.UpdatedETags[*in.Key] = `"` + etag + `"`
	return out, nil
}

func (m *S3API) CreateMultipartUpload(ctx context.Context, in *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("key is required")
	}
	out := &s3v2.CreateMultipartUploadOutput{
		Bucket:   in.Bucket,
		Key:      in.Key,
		UploadId: &uploadID,
	}
	m.MPUCreated = true
	return out, nil
}

func (m *S3API) UploadPart(ctx context.Context, in *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("UploaderID is missing or invalid")
	}
	if in.PartNumber == nil {
		return nil, errors.New("PartNumber is required")
	}
	etag, err := md5hex(in.Body)
	if err != nil {
		return nil, err
	}
	out := &s3v2.UploadPartOutput{
		ETag: &etag,
	}
	m.parts.Store(*in.PartNumber, etag)
	return out, nil
}

func (m *S3API) UploadPartCopy(ctx context.Context, in *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("UploaderID is missing or invalid")
	}
	if in.PartNumber == nil {
		return nil, errors.New("PartNumber is required")
	}
	if in.CopySource == nil {
		return nil, errors.New("CopySource is required")
	}
	copySourceDecoded, err := url.QueryUnescape(*in.CopySource)
	if err != nil {
		return nil, fmt.Errorf("parsing copy source: %w", err)
	}
	srcBucket, srcKey, _ := strings.Cut(copySourceDecoded, "/")
	if srcBucket != m.bucket {
		return nil, &types.NoSuchBucket{}
	}
	srcObj, err := m.getObject(&srcKey)
	if err != nil {
		return nil, err
	}
	if in.CopySourceRange == nil {
		return nil, errors.New("CopySourceRange is required")
	}
	start, end, err := parseByteRange(*in.CopySourceRange)
	if err != nil {
		return nil, err
	}
	etag, err := md5hex(bytes.NewReader(srcObj.Body[start : end+1]))
	if err != nil {
		return nil, err
	}
	out := &s3v2.UploadPartCopyOutput{
		CopyPartResult: &types.CopyPartResult{ETag: aws.String(etag)},
	}
	m.parts.Store(*in.PartNumber, etag)
	return out, nil
}

func (m *S3API) CompleteMultipartUpload(ctx context.Context, in *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("invalid uploader id")
	}
	if in.MultipartUpload == nil {
		return nil, errors.New("multipart upload is required")
	}
	etags := make([][]byte, len(in.MultipartUpload.Parts))
	for i, p := range in.MultipartUpload.Parts {
		if p.PartNumber == nil {
			return nil, errors.New("nil partnumber")
		}
		if p.ETag == nil {
			return nil, errors.New("nil etag in upload parts")
		}
		tag := m.PartETag(*p.PartNumber)
		if tag == "" {
			return nil, fmt.Errorf("no part with number %d", *p.PartNumber)
		}
		if tag != *p.ETag {
			return nil, fmt.Errorf("etags don't match for part number %d", *p.PartNumber)
		}
		tagDecode, err := hex.DecodeString(tag)
		if err != nil {
			return nil, err
		}
		etags[i] = tagDecode
	}
	etag, err := md5hex(bytes.NewReader(bytes.Join(etags, nil)))
	if err != nil {
		return nil, err
	}
	etag = fmt.Sprintf(`"%s-%d"`, etag, len(etags))
	out := &s3v2.CompleteMultipartUploadOutput{
		Bucket: in.Bucket,
		Key:    in.Key,
		ETag:   aws.String(etag),
	}
	m.UpdatedETags[*in.Key] = etag
	m.MPUComplete = true
	return out, nil
}

func (m *S3API) AbortMultipartUpload(ctx context.Context, in *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("UploaderID is required")
	}

	out := &s3v2.AbortMultipartUploadOutput{}
	m.MPUAborted = true
	return out, nil
}

func (m *S3API) CopyObject(ctx context.Context, in *s3v2.CopyObjectInput, opts ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	if m.CopyObjectFunc != nil {
		return m.CopyObjectFunc(ctx, in, opts...)
	}
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("Key is required")
	}
	if in.CopySource == nil {
		return nil, errors.New("CopySource is required")
	}
	copySourceDecoded, err := url.QueryUnescape(*in.CopySource)
	if err != nil {
		return nil, fmt.Errorf("parsing copy source: %w", err)
	}
	srcBucket, srcKey, _ := strings.Cut(copySourceDecoded, "/")
	if srcBucket != m.bucket {
		return nil, &types.NoSuchBucket{}
	}
	srcObj, err := m.getObject(&srcKey)
	if err != nil {
		return nil, err
	}
	etag, err := md5hex(bytes.NewReader(srcObj.Body))
	if err != nil {
		return nil, err
	}
	out := &s3v2.CopyObjectOutput{
		CopyObjectResult: &types.CopyObjectResult{ETag: aws.String(etag)},
	}
	m.UpdatedETags[*in.Key] = `"` + etag + `"` // etag is quoted string
	return out, nil
}

func (m *S3API) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	if _, err := m.getObject(in.Key); err != nil {
		return nil, &types.NoSuchKey{}
	}
	out := &s3v2.DeleteObjectOutput{}
	m.Deleted[*in.Key] = true
	return out, nil
}

func (m *S3API) PartCount() int {
	num := 0
	m.parts.Range(func(_, _ any) bool {
		num++
		return true
	})
	return num
}

func (m *S3API) PartETag(num int32) string {
	tag, exists := m.parts.Load(num)
	if !exists {
		return ""
	}
	return tag.(string)
}

func (m *S3API) objectKeys() []string {
	keys := make([]string, 0, len(m.objects))
	for k := range m.objects {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func (m *S3API) bucketOK(b *string) error {
	if !eql(m.bucket, b) {
		return &types.NoSuchBucket{}
	}
	return nil
}

func (m *S3API) getObject(k *string) (*Object, error) {
	if k == nil {
		return nil, errors.New("object key is required")
	}
	obj, ok := m.objects[*k]
	if !ok {
		return nil, &types.NoSuchKey{}
	}
	return obj, nil
}

var _ s3.S3API = (*S3API)(nil)

type Object struct {
	Key           string
	Body          []byte
	LastModified  time.Time
	ContentLength int64
}

// func GenObjects(seed uint64, objCount int, keyPrefix string, depth int, maxFileSize int64) map[string]*Object {
// 	if depth < 1 {
// 		depth = 1
// 	}
// 	if objCount < 1 {
// 		objCount = 1
// 	}
// 	if maxFileSize < 1 {
// 		maxFileSize = 1
// 	}
// 	gen := rand.New(rand.NewSource(seed))
// 	objects := make(map[string]*Object, objCount)
// 	keys := make(map[string]struct{}, objCount)
// 	dirs := map[string]struct{}{".": {}}
// 	genKey := func() string {
// 		for {
// 			// loop until we get a uniqu key
// 			var dir string
// 			switch {
// 			case depth == 1:
// 				dir = "."
// 			case gen.Intn(4) > 0:
// 				// use an existing directory
// 				for dir = range dirs {
// 					break
// 				}
// 			default:
// 				// create a new directory
// 				dirParts := make([]string, gen.Intn(depth))
// 				for j := range dirParts {
// 					dirParts[j] = randPathPart(gen, 2, 8)
// 				}
// 				dir = path.Join(dirParts...)
// 			}
// 			key := path.Join(dir, randPathPart(gen, 5, 12))
// 			if _, exists := keys[key]; !exists {
// 				keys[key] = struct{}{}
// 				dirs[dir] = struct{}{}
// 				return key
// 			}
// 		}
// 	}
// 	for i := 0; i < objCount; i++ {
// 		key := path.Join(keyPrefix, genKey())
// 		objects[key] = &Object{
// 			ContentLength: gen.Int63n(maxFileSize + 1),
// 			LastModified:  time.Unix(1711391789-int64(gen.Intn(31536000)), 0),
// 		}
// 	}
// 	return objects
// }

// func randPathPart(genr *rand.Rand, minSize, maxSize int) string {
// 	const chars = `abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_.`
// 	const lenChars = len(chars)
// 	size := minSize
// 	if size < 1 {
// 		size = 1
// 	}
// 	if maxSize > size {
// 		size += genr.Intn(maxSize - size + 1)
// 	}
// 	out := ""
// 	for i := 0; i < size; i++ {
// 		var next byte
// 		for {
// 			next = chars[genr.Intn(lenChars)]
// 			if next == '.' && i > 0 && out[i-1] == '.' {
// 				// dont allow '..'
// 				continue // try again
// 			}
// 			if size == 1 && next == '.' {
// 				continue
// 			}
// 			break
// 		}
// 		out += string(next)
// 	}
// 	return out
// }

func md5hex(r io.Reader) (string, error) {
	digester := md5.New()
	if _, err := io.Copy(digester, r); err != nil {
		return "", err
	}
	return hex.EncodeToString(digester.Sum(nil)), nil
}

func eql[T comparable](expect T, ptr *T) bool {
	if ptr == nil {
		return false
	}
	return *ptr == expect
}

func parseByteRange(brange string) (start int64, end int64, err error) {
	if !byteRangeRE.MatchString(brange) {
		err = fmt.Errorf("invalid bytes range: %s", brange)
		return
	}
	brange = strings.TrimPrefix(brange, "bytes=")
	a, b, _ := strings.Cut(brange, "-")
	start, err = strconv.ParseInt(a, 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid bytes range: %w", err)
		return
	}
	end, err = strconv.ParseInt(b, 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid bytes range: %w", err)
	}
	if start < 0 || start > end {
		err = fmt.Errorf("invalid bytes range: %s", brange)
		return
	}
	return
}

func ETag(b []byte, psize int64) string {
	r := bytes.NewReader(b)
	if len(b) < 5*1024*1024 {
		// less then 5MB, min part size
		etag, err := md5hex(r)
		if err != nil {
			panic(err)
		}
		return etag
	}
	digester := md5.New()
	numParts := 0
	for {
		partDigester := md5.New()
		n, err := io.CopyN(partDigester, r, psize)
		if err != nil && !errors.Is(err, io.EOF) {
			panic(err)
		}
		if n > 0 {
			numParts++
			_, err := digester.Write(partDigester.Sum(nil))
			if err != nil {
				panic(err)
			}
		}
		if errors.Is(err, io.EOF) {
			break
		}
	}
	sum := hex.EncodeToString(digester.Sum(nil))
	return fmt.Sprintf(`"%s-%d"`, sum, numParts)
}
