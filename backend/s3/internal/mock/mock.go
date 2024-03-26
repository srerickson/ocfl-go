package mock

import (
	"bytes"
	"context"
	"io"
	"path"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	s3v2 "github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/aws-sdk-go/aws"
	"github.com/carlmjohnson/be"
	"github.com/srerickson/ocfl-go"
	"github.com/srerickson/ocfl-go/backend/s3"
	"golang.org/x/exp/rand"
)

type S3Func[input any, output any] func(context.Context, input, ...func(*s3v2.Options)) (output, error)

type S3API struct {
	Head        S3Func[*s3v2.HeadObjectInput, *s3v2.HeadObjectOutput]
	Get         S3Func[*s3v2.GetObjectInput, *s3v2.GetObjectOutput]
	List        S3Func[*s3v2.ListObjectsV2Input, *s3v2.ListObjectsV2Output]
	Put         S3Func[*s3v2.PutObjectInput, *s3v2.PutObjectOutput]
	PutPart     S3Func[*s3v2.UploadPartInput, *s3v2.UploadPartOutput]
	PutPartCopy S3Func[*s3v2.UploadPartCopyInput, *s3v2.UploadPartCopyOutput]
	Copy        S3Func[*s3v2.CopyObjectInput, *s3v2.CopyObjectOutput]
	Delete      S3Func[*s3v2.DeleteObjectInput, *s3v2.DeleteObjectOutput]
	CreateMPU   S3Func[*s3v2.CreateMultipartUploadInput, *s3v2.CreateMultipartUploadOutput]
	CompleteMPU S3Func[*s3v2.CompleteMultipartUploadInput, *s3v2.CompleteMultipartUploadOutput]
	AbortMPU    S3Func[*s3v2.AbortMultipartUploadInput, *s3v2.AbortMultipartUploadOutput]
}

func OpenFileAPI(t *testing.T, bucket string, objects ...*Object) *S3API {
	mockObjects := objectMap(objects...)
	getObj := func(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
		be.Nonzero(t, param.Bucket)
		be.Nonzero(t, param.Key)
		if *param.Bucket != bucket {
			return nil, &types.NoSuchBucket{}
		}
		obj := mockObjects[*param.Key]
		if obj == nil {
			return nil, &types.NoSuchKey{}
		}
		return &s3v2.GetObjectOutput{
			Body:          io.NopCloser(bytes.NewBuffer(obj.Body)),
			ContentLength: aws.Int64(int64(len(obj.Body))),
			LastModified:  aws.Time(obj.LastModified),
		}, nil
	}
	return &S3API{Get: getObj}
}

func ReadDirAPI(t *testing.T, bucket string, objects ...*Object) *S3API {
	sortObjects(objects)
	listObjs := func(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
		t.Helper()
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Delimiter)
		be.Equal(t, "/", *in.Delimiter)
		maxkeys := int32(1000)
		if in.MaxKeys != nil {
			maxkeys = *in.MaxKeys
		}
		be.Equal(t, 1000, maxkeys)
		prefix := ""
		if in.Prefix != nil {
			be.True(t, strings.HasSuffix(*in.Prefix, "/"))
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
		for i, object := range objects {
			if in.ContinuationToken != nil && object.Key <= *in.ContinuationToken {
				continue
			}
			if !strings.HasPrefix(object.Key, prefix) {
				continue
			}
			keySuffix := strings.TrimPrefix(object.Key, prefix)
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
					Key:          aws.String(object.Key),
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
			haveMoreKeys := i < len(objects)-1
			if haveMoreKeys && keyCount >= 1000 || numCommonPrefixes >= 1000 {
				// that's all we can include,
				out.IsTruncated = aws.Bool(true)
				out.NextContinuationToken = aws.String(object.Key)
				break
			}
		}
		return out, nil
	}
	return &S3API{List: listObjs}
}

func WriteAPI(t *testing.T, bucket string) *S3API {
	uploadID := "mock-upload-id"
	parts := sync.Map{}
	put := func(ctx context.Context, in *s3v2.PutObjectInput, opts ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Key)
		be.Nonzero(t, in.Body)
		sum, err := etag(in.Body)
		be.NilErr(t, err)
		out := &s3v2.PutObjectOutput{
			ETag: &sum,
		}
		return out, nil
	}
	createMPU := func(ctx context.Context, in *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Key)
		out := &s3v2.CreateMultipartUploadOutput{
			Bucket:   in.Bucket,
			Key:      in.Key,
			UploadId: &uploadID,
		}
		return out, nil
	}
	putPart := func(ctx context.Context, in *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Key)
		be.Nonzero(t, in.UploadId)
		be.Equal(t, uploadID, *in.UploadId)
		be.Nonzero(t, in.PartNumber)
		sum, err := etag(in.Body)
		parts.Store(*in.PartNumber, sum)
		be.NilErr(t, err)
		out := &s3v2.UploadPartOutput{
			ETag: &sum,
		}
		return out, nil
	}
	completeMPU := func(ctx context.Context, in *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Key)
		be.Nonzero(t, in.UploadId)
		be.Equal(t, uploadID, *in.UploadId)
		be.Nonzero(t, in.MultipartUpload)
		for _, p := range in.MultipartUpload.Parts {
			be.Nonzero(t, p.PartNumber)
			be.Nonzero(t, p.ETag)
			tag, exists := parts.Load(*p.PartNumber)
			be.True(t, exists)
			be.Equal(t, *p.ETag, tag.(string))
		}
		out := &s3v2.CompleteMultipartUploadOutput{
			Bucket: in.Bucket,
			Key:    in.Key,
			ETag:   aws.String("computed-etag"),
		}
		return out, nil
	}
	abortMPU := func(ctx context.Context, in *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
		be.Nonzero(t, in.Bucket)
		be.Equal(t, bucket, *in.Bucket)
		be.Nonzero(t, in.Key)
		be.Nonzero(t, in.UploadId)
		be.Equal(t, uploadID, *in.UploadId)
		out := &s3v2.AbortMultipartUploadOutput{}
		return out, nil
	}
	return &S3API{
		Put:         put,
		PutPart:     putPart,
		CreateMPU:   createMPU,
		CompleteMPU: completeMPU,
		AbortMPU:    abortMPU,
	}
}

type Object struct {
	Key           string
	Body          []byte
	LastModified  time.Time
	ContentLength int64
}

func sortObjects(objects []*Object) {
	sort.Slice(objects, func(i, j int) bool {
		return objects[i].Key < objects[j].Key
	})
}

func objectMap(objs ...*Object) map[string]*Object {
	mockObjects := make(map[string]*Object, len(objs))
	for _, obj := range objs {
		mockObjects[obj.Key] = obj
	}
	return mockObjects
}

func (m *S3API) HeadObject(ctx context.Context, param *s3v2.HeadObjectInput, opts ...func(*s3v2.Options)) (*s3v2.HeadObjectOutput, error) {
	return m.Head(ctx, param, opts...)
}

func (m *S3API) GetObject(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
	return m.Get(ctx, param, opts...)
}

func (m *S3API) ListObjectsV2(ctx context.Context, param *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
	return m.List(ctx, param, opts...)
}

func (m *S3API) PutObject(ctx context.Context, param *s3v2.PutObjectInput, opts ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
	return m.Put(ctx, param, opts...)
}
func (m *S3API) CreateMultipartUpload(ctx context.Context, param *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
	return m.CreateMPU(ctx, param, opts...)
}

func (m *S3API) UploadPart(ctx context.Context, param *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
	return m.PutPart(ctx, param, opts...)
}
func (m *S3API) UploadPartCopy(ctx context.Context, param *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	return m.PutPartCopy(ctx, param, opts...)
}
func (m *S3API) CompleteMultipartUpload(ctx context.Context, param *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
	return m.CompleteMPU(ctx, param, opts...)
}

func (m *S3API) AbortMultipartUpload(ctx context.Context, param *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
	return m.AbortMPU(ctx, param, opts...)
}

func (m *S3API) CopyObject(ctx context.Context, param *s3v2.CopyObjectInput, opts ...func(*s3v2.Options)) (*s3v2.CopyObjectOutput, error) {
	return m.Copy(ctx, param, opts...)
}

func (m *S3API) DeleteObject(ctx context.Context, param *s3v2.DeleteObjectInput, opts ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
	return m.Delete(ctx, param, opts...)
}

var _ s3.S3API = (*S3API)(nil)

func GenObjects(seed uint64, objCount int, keyPrefix string, depth int, maxFileSize int64) []*Object {
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
	objects := make([]*Object, objCount)
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
		objects[i] = &Object{
			Key:           path.Join(keyPrefix, genKey()),
			ContentLength: gen.Int63n(maxFileSize + 1),
			LastModified:  time.Unix(1711391789-int64(gen.Intn(31536000)), 0),
		}
	}
	sortObjects(objects)
	return objects
}

func randPathPart(src *rand.Rand, minSize, maxSize int) string {
	const chars = `abcdefghijklmnopqrstuvwzyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890-_.`
	const lenChars = len(chars)
	size := minSize
	if size < 1 {
		size = 1
	}
	if maxSize > size {
		size += src.Intn(maxSize - size + 1)
	}
	out := ""
	for i := 0; i < size; i++ {
		var next byte
		for {
			next = chars[src.Intn(lenChars)]
			if size == 2 && i > 0 && out[i-1] == '.' && next == '.' {
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

func etag(r io.Reader) (string, error) {
	digester := ocfl.NewDigester(ocfl.MD5)
	if _, err := io.Copy(digester, r); err != nil {
		return "", err
	}
	return digester.String(), nil
}
