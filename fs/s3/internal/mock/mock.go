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

	"github.com/aws/aws-sdk-go-v2/aws"
	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
	"github.com/srerickson/ocfl-go/fs/s3"
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

// WithNotFoundStyle sets the error shape m returns for a request naming a key
// the bucket does not hold, and returns m so it can be chained onto New:
//
//	mock.New(bucket, objs...).WithNotFoundStyle(mock.NotFoundStyleGeneric)
//
// The default is [NotFoundStyleAWS]. Set it before serving any request: the
// field is read without the state lock.
func (m *S3API) WithNotFoundStyle(style NotFoundStyle) *S3API {
	m.notFoundStyle = style
	return m
}

type S3API struct {
	// UpdatedETags, Deleted and the MPU flags are written by the mock's
	// handlers and read by tests. Handlers may run on the uploader's
	// goroutines while the test reads from its own, so read them through
	// the accessors (UpdatedETag, WasDeleted, MPUCreatedFlag, ...) rather
	// than directly: the accessor takes the state mutex, the bare field
	// read does not. The fields stay exported for construction-time
	// compatibility, but direct reads are only safe before the mock has
	// served its first concurrent request.
	UpdatedETags map[string]string
	// Deleted records every key a delete request targeted. It is a call
	// log, not the bucket's state: a deleted key is also removed from the
	// bucket, so prefer asserting through HeadObject, GetObject or
	// ListObjectsV2 unless the test is specifically about which requests
	// were issued.
	Deleted     map[string]bool
	MPUCreated  bool
	MPUAborted  bool
	MPUComplete bool

	CopyObjectFunc func(context.Context, *s3v2.CopyObjectInput, ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error)

	parts   sync.Map
	bucket  string
	objects map[string]*Object

	// notFoundStyle selects the error shape returned for a missing key.
	// It is set at construction and never written afterwards, so it needs
	// no lock. See errors.go.
	notFoundStyle NotFoundStyle

	// mu guards objects, UpdatedETags, Deleted and the MPU flags. parts
	// keeps its own sync.Map and log its own mutex, so a handler never
	// holds mu while touching either — no lock ordering to reason about.
	mu sync.Mutex

	// log records every request served, so tests can assert request shape
	// without wrapping the mock. See calls.go.
	log callLog
}

func (m *S3API) HeadObject(ctx context.Context, in *s3v2.HeadObjectInput, opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	m.log.recordKey("HeadObject", in.Key)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	obj, err := m.getObject(in.Key)
	if errors.Is(err, errMissingKey) {
		// A HEAD response has no body, so a real endpoint has nothing to
		// deserialize a code from and the SDK derives one from the 404
		// status: NotFound, not NoSuchKey.
		err = m.notFound(&types.NotFound{Message: aws.String("Not Found")})
	}
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
	m.log.recordKey("GetObject", in.Key)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	// Clone under the lock: the caller reads the returned body after this
	// method returns, and a concurrent PutObject to the same key replaces
	// m.objects — the read of obj.Body must not race that write, and the
	// returned buffer must stay valid if it does.
	m.mu.Lock()
	obj, err := m.getObjectLocked(in.Key)
	if err != nil {
		m.mu.Unlock()
		return nil, m.noSuchKeyErr(err)
	}
	body := obj.Body
	lastMod := obj.LastModified
	contentLength := int64(len(obj.Body))
	// Handle Range header for partial reads
	if in.Range != nil && *in.Range != "" {
		start, end, err := parseGetObjectRange(*in.Range, contentLength)
		if err != nil {
			m.mu.Unlock()
			return nil, err
		}
		body = obj.Body[start : end+1]
		contentLength = end - start + 1
	}
	body = bytes.Clone(body)
	m.mu.Unlock()
	return &s3v2.GetObjectOutput{
		Body:          io.NopCloser(bytes.NewBuffer(body)),
		ContentLength: aws.Int64(contentLength),
		LastModified:  aws.Time(lastMod),
	}, nil
}

func (m *S3API) ListObjectsV2(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	if prefix := aws.ToString(in.Prefix); prefix != "" {
		m.log.record("ListObjectsV2", prefix)
	} else {
		// A bucket-wide listing names no prefix; recording "" as a key would
		// make KeysFor("ListObjectsV2") report a key that was never sent.
		m.log.record("ListObjectsV2")
	}
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
	// The listing must be a consistent snapshot of the bucket, so the state
	// lock is held for the whole scan: a concurrent PutObject or DeleteObject
	// between objectKeys() and the per-key read would otherwise be a data
	// race, not just a stale listing.
	m.mu.Lock()
	defer m.mu.Unlock()
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
	m.log.recordKey("PutObject", in.Key)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("key is required")
	}
	if in.Body == nil {
		return nil, errors.New("body is required")
	}
	body, err := io.ReadAll(in.Body)
	if err != nil {
		return nil, err
	}
	// A declared Content-Length is a promise about the body, so hold the
	// request to it instead of storing whatever arrived. See incompleteBody.
	if in.ContentLength != nil && *in.ContentLength != int64(len(body)) {
		return nil, incompleteBody(*in.ContentLength, int64(len(body)))
	}
	etag, err := md5hex(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	out := &s3v2.PutObjectOutput{
		ETag: &etag,
	}
	// Materialize the object exactly like real S3: a subsequent HeadObject,
	// GetObject or ListObjectsV2 must find it. Without this the mock cannot
	// round-trip a Write followed by an OpenFile, so a test against it can
	// only assert which requests were sent, never what the bucket holds.
	m.mu.Lock()
	m.objects[*in.Key] = &Object{
		Key:           *in.Key,
		Body:          body,
		ContentLength: int64(len(body)),
		LastModified:  time.Now(),
	}
	m.UpdatedETags[*in.Key] = `"` + etag + `"`
	m.mu.Unlock()
	return out, nil
}

func (m *S3API) CreateMultipartUpload(ctx context.Context, in *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	m.log.recordKey("CreateMultipartUpload", in.Key)
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
	m.mu.Lock()
	m.MPUCreated = true
	m.mu.Unlock()
	return out, nil
}

func (m *S3API) UploadPart(ctx context.Context, in *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	m.log.recordKey("UploadPart", in.Key)
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
	m.log.recordKey("UploadPartCopy", in.Key)
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
		return nil, m.noSuchKeyErr(err)
	}
	if in.CopySourceRange == nil {
		return nil, errors.New("CopySourceRange is required")
	}
	start, end, err := parseByteRange(*in.CopySourceRange)
	if err != nil {
		return nil, err
	}
	// Copy the source range under the state lock, as in CopyObject: the
	// backend issues UploadPartCopy concurrently with other requests, and
	// slicing srcObj.Body must not race a PutObject replacing the source.
	m.mu.Lock()
	srcRange := bytes.Clone(srcObj.Body[start : end+1])
	m.mu.Unlock()
	etag, err := md5hex(bytes.NewReader(srcRange))
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
	m.log.recordKey("CompleteMultipartUpload", in.Key)
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
	m.mu.Lock()
	m.UpdatedETags[*in.Key] = etag
	m.MPUComplete = true
	m.mu.Unlock()
	return out, nil
}

func (m *S3API) AbortMultipartUpload(ctx context.Context, in *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	m.log.recordKey("AbortMultipartUpload", in.Key)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.UploadId == nil || *in.UploadId != uploadID {
		return nil, errors.New("UploaderID is required")
	}

	out := &s3v2.AbortMultipartUploadOutput{}
	m.mu.Lock()
	m.MPUAborted = true
	m.mu.Unlock()
	return out, nil
}

func (m *S3API) CopyObject(ctx context.Context, in *s3v2.CopyObjectInput, opts ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	m.log.recordKey("CopyObject", in.Key)
	if m.CopyObjectFunc != nil {
		return m.CopyObjectFunc(ctx, in, opts...)
	}
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("key is required")
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
		return nil, m.noSuchKeyErr(err)
	}
	// Copy the body under the state lock: a concurrent PutObject to the
	// source key would otherwise race the read in md5hex.
	m.mu.Lock()
	srcBody := bytes.Clone(srcObj.Body)
	m.mu.Unlock()
	etag, err := md5hex(bytes.NewReader(srcBody))
	if err != nil {
		return nil, err
	}
	out := &s3v2.CopyObjectOutput{
		CopyObjectResult: &types.CopyObjectResult{ETag: aws.String(etag)},
	}
	m.mu.Lock()
	m.UpdatedETags[*in.Key] = `"` + etag + `"` // etag is quoted string
	m.mu.Unlock()
	return out, nil
}

func (m *S3API) DeleteObject(ctx context.Context, in *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	m.log.recordKey("DeleteObject", in.Key)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Key == nil {
		return nil, errors.New("key is required")
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	// DeleteObject is idempotent: a real endpoint answers 204 whether or
	// not the key was there, and reports nothing about which it was. A
	// mock that errored on a missing key would let the s3 backend's Remove
	// look strict under test while returning nil in production.
	m.deleteKeyLocked(*in.Key)
	return &s3v2.DeleteObjectOutput{}, nil
}

// MaxDeleteBatch is the most keys DeleteObjects accepts in one request. It is
// a fixed S3 API limit, not a tunable, and the mock enforces it: a caller that
// batches a listing page must not be able to send an oversized request that
// only a real endpoint rejects.
const MaxDeleteBatch = 1000

func (m *S3API) DeleteObjects(ctx context.Context, in *s3v2.DeleteObjectsInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectsOutput, error) {
	// Record every key the request carried, so a test can tell one batch of
	// n keys from n separate DeleteObject calls -- KeysFor is identical for
	// the two, KeyBatchesFor is not.
	var keys []string
	if in.Delete != nil {
		for _, obj := range in.Delete.Objects {
			keys = append(keys, aws.ToString(obj.Key))
		}
	}
	m.log.record("DeleteObjects", keys...)
	if err := m.bucketOK(in.Bucket); err != nil {
		return nil, err
	}
	if in.Delete == nil || len(in.Delete.Objects) == 0 {
		return nil, errors.New("Delete with at least one object is required")
	}
	if len(in.Delete.Objects) > MaxDeleteBatch {
		return nil, &smithy.GenericAPIError{
			Code:    "MalformedXML",
			Message: fmt.Sprintf("a delete request carries at most %d keys, got %d", MaxDeleteBatch, len(in.Delete.Objects)),
		}
	}
	quiet := aws.ToBool(in.Delete.Quiet)
	out := &s3v2.DeleteObjectsOutput{}
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, obj := range in.Delete.Objects {
		if obj.Key == nil {
			return nil, errors.New("object key is required")
		}
		// Each key deletes the way DeleteObject does: idempotently, whether
		// or not the bucket held it.
		m.deleteKeyLocked(*obj.Key)
		if !quiet {
			out.Deleted = append(out.Deleted, types.DeletedObject{Key: obj.Key})
		}
	}
	return out, nil
}

// deleteKeyLocked removes the object from the bucket and records the
// deletion. The caller must hold m.mu.
//
// Removing it from m.objects is what makes a deletion observable the way a
// real store's is: the key stops appearing in ListObjectsV2 and stops
// answering HeadObject and GetObject. The Deleted map is call bookkeeping
// kept alongside it — useful for asserting that a specific key was targeted,
// and for distinguishing "never deleted" from "deleted and then rewritten" —
// but a test that only consults Deleted is checking which requests were sent,
// not what the bucket now contains.
func (m *S3API) deleteKeyLocked(key string) {
	delete(m.objects, key)
	m.Deleted[key] = true
}

func (m *S3API) PartCount() int {
	num := 0
	m.parts.Range(func(_, _ any) bool {
		num++
		return true
	})
	return num
}

// WasDeleted reports whether a delete request targeted key. Prefer this over
// reading the Deleted map directly: the map is written by handlers that may
// be running on the uploader's goroutines, and the accessor takes the state
// mutex.
func (m *S3API) WasDeleted(key string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Deleted[key]
}

// UpdatedETag returns the etag recorded for key, or "" if the mock never
// completed a write to it. Prefer this over reading UpdatedETags directly,
// for the same reason as WasDeleted.
func (m *S3API) UpdatedETag(key string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.UpdatedETags[key]
}

// MPUCreatedFlag, MPUAbortedFlag and MPUCompleteFlag report the corresponding
// MPU flag under the state mutex; prefer them over the bare fields.
func (m *S3API) MPUCreatedFlag() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MPUCreated
}

func (m *S3API) MPUAbortedFlag() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MPUAborted
}

func (m *S3API) MPUCompleteFlag() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.MPUComplete
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
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.getObjectLocked(k)
}

// getObjectLocked looks up an object; the caller must hold m.mu. Callers
// that mutate or hand out the object's Body should copy it before releasing
// the lock (see CopyObject).
func (m *S3API) getObjectLocked(k *string) (*Object, error) {
	if k == nil {
		return nil, errors.New("object key is required")
	}
	obj, ok := m.objects[*k]
	if !ok {
		return nil, errMissingKey
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

// parseGetObjectRange parses Range headers for GetObject which can be:
// - "bytes=start-end" (both specified)
// - "bytes=start-" (from start to end of file)
// - "bytes=-suffix" (last N bytes) - not commonly used
func parseGetObjectRange(brange string, totalSize int64) (start int64, end int64, err error) {
	if !strings.HasPrefix(brange, "bytes=") {
		err = fmt.Errorf("invalid bytes range: %s", brange)
		return
	}
	brange = strings.TrimPrefix(brange, "bytes=")
	a, b, _ := strings.Cut(brange, "-")

	// Handle "bytes=-suffix" (last N bytes)
	if a == "" {
		suffix, parseErr := strconv.ParseInt(b, 10, 64)
		if parseErr != nil {
			err = fmt.Errorf("invalid bytes range: %w", parseErr)
			return
		}
		start = totalSize - suffix
		if start < 0 {
			start = 0
		}
		end = totalSize - 1
		return
	}

	start, err = strconv.ParseInt(a, 10, 64)
	if err != nil {
		err = fmt.Errorf("invalid bytes range: %w", err)
		return
	}

	// Handle "bytes=start-" (from start to end)
	if b == "" {
		end = totalSize - 1
	} else {
		end, err = strconv.ParseInt(b, 10, 64)
		if err != nil {
			err = fmt.Errorf("invalid bytes range: %w", err)
			return
		}
	}

	if start < 0 || start > end || start >= totalSize {
		err = fmt.Errorf("invalid bytes range: bytes=%s-%s", a, b)
		return
	}
	if end >= totalSize {
		end = totalSize - 1
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
