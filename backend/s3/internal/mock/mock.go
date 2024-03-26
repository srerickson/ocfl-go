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

type S3Func[Tin any, Tout any] func(context.Context, Tin, ...func(*s3v2.Options)) (Tout, error)

type S3API struct {
	Head        S3Func[*s3v2.HeadObjectInput, *s3v2.HeadObjectOutput]
	Get         S3Func[*s3v2.GetObjectInput, *s3v2.GetObjectOutput]
	List        S3Func[*s3v2.ListObjectsV2Input, *s3v2.ListObjectsV2Output]
	Put         S3Func[*s3v2.PutObjectInput, *s3v2.PutObjectOutput]
	Upload      S3Func[*s3v2.UploadPartInput, *s3v2.UploadPartOutput]
	UploadCopy  S3Func[*s3v2.UploadPartCopyInput, *s3v2.UploadPartCopyOutput]
	Copy        S3Func[*s3v2.CopyObjectInput, *s3v2.CopyObjectOutput]
	Delete      S3Func[*s3v2.DeleteObjectInput, *s3v2.DeleteObjectOutput]
	CreateMPU   S3Func[*s3v2.CreateMultipartUploadInput, *s3v2.CreateMultipartUploadOutput]
	CompleteMPU S3Func[*s3v2.CompleteMultipartUploadInput, *s3v2.CompleteMultipartUploadOutput]
	AbortMPU    S3Func[*s3v2.AbortMultipartUploadInput, *s3v2.AbortMultipartUploadOutput]
}

func OpenFileAPI(bucket string, objects ...*Object) *S3API {
	mockObjects := objectMap(objects)
	getObj := func(ctx context.Context, param *s3v2.GetObjectInput, opts ...func(*s3v2.Options)) (*s3v2.GetObjectOutput, error) {
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

func ReadDirAPI(bucket string, objects ...*Object) *S3API {
	sortObjects(objects)
	listObjs := func(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
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
			if haveMoreKeys && keyCount >= int(maxkeys) || numCommonPrefixes >= int(maxkeys) {
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

func WriteAPI(bucket string) *S3API {
	uploadID := "mock-upload-id"
	parts := sync.Map{}
	put := func(ctx context.Context, in *s3v2.PutObjectInput, opts ...func(*s3v2.Options)) (*s3v2.PutObjectOutput, error) {
		sum, err := etag(in.Body)
		if err != nil {
			return nil, err
		}
		out := &s3v2.PutObjectOutput{
			ETag: &sum,
		}
		return out, nil
	}
	createMPU := func(ctx context.Context, in *s3v2.CreateMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CreateMultipartUploadOutput, error) {
		out := &s3v2.CreateMultipartUploadOutput{
			Bucket:   in.Bucket,
			Key:      in.Key,
			UploadId: &uploadID,
		}
		return out, nil
	}
	upload := func(ctx context.Context, in *s3v2.UploadPartInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartOutput, error) {
		sum, err := etag(in.Body)
		if err != nil {
			return nil, err
		}
		parts.Store(*in.PartNumber, sum)
		out := &s3v2.UploadPartOutput{
			ETag: &sum,
		}
		return out, nil
	}
	completeMPU := func(ctx context.Context, in *s3v2.CompleteMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.CompleteMultipartUploadOutput, error) {
		if in.UploadId == nil || *in.UploadId != uploadID {
			return nil, errors.New("invalid uploader id")
		}
		if in.MultipartUpload == nil {
			return nil, errors.New("no uploads")
		}
		for _, p := range in.MultipartUpload.Parts {
			if p.PartNumber == nil {
				return nil, errors.New("nil partnumber")
			}
			if p.ETag == nil {
				return nil, errors.New("nil etag in upload parts")
			}
			tag, exists := parts.Load(*p.PartNumber)
			if !exists {
				return nil, fmt.Errorf("unknown part number %d", *p.PartNumber)
			}
			if tag != *p.ETag {
				return nil, fmt.Errorf("etags don't match for part number %d", *p.PartNumber)
			}
		}
		out := &s3v2.CompleteMultipartUploadOutput{
			Bucket: in.Bucket,
			Key:    in.Key,
			ETag:   aws.String("computed-etag"),
		}
		return out, nil
	}
	abortMPU := func(ctx context.Context, in *s3v2.AbortMultipartUploadInput, opts ...func(*s3v2.Options)) (*s3v2.AbortMultipartUploadOutput, error) {
		if in.UploadId == nil || *in.UploadId != uploadID {
			return nil, errors.New("invalid uploader id")
		}
		out := &s3v2.AbortMultipartUploadOutput{}
		return out, nil
	}
	return &S3API{
		Put:         put,
		Upload:      upload,
		CreateMPU:   createMPU,
		CompleteMPU: completeMPU,
		AbortMPU:    abortMPU,
	}
}

func RemoveAllAPI(bucket string, objects ...*Object) *S3API {
	sortObjects(objects)
	toDelete := map[string]bool{}
	objMap := objectMap(objects)
	listObjs := func(ctx context.Context, in *s3v2.ListObjectsV2Input, opts ...func(*s3v2.Options)) (*s3v2.ListObjectsV2Output, error) {
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
		for i, object := range objects {
			if in.ContinuationToken != nil && object.Key <= *in.ContinuationToken {
				continue
			}
			if !strings.HasPrefix(object.Key, prefix) {
				continue
			}
			toDelete[object.Key] = false
			cont := types.Object{
				Key:          aws.String(object.Key),
				Size:         aws.Int64(object.ContentLength),
				LastModified: aws.Time(object.LastModified),
			}
			out.Contents = append(out.Contents, cont)
			keyCount := len(out.Contents)
			out.KeyCount = aws.Int32(int32(keyCount))
			haveMoreKeys := i < len(objects)-1
			if haveMoreKeys && keyCount >= int(maxkeys) {
				// that's all we can include,
				out.IsTruncated = aws.Bool(true)
				out.NextContinuationToken = aws.String(object.Key)
				break
			}
		}
		return out, nil
	}
	delete := func(_ context.Context, in *s3v2.DeleteObjectInput, _ ...func(*s3v2.Options)) (*s3v2.DeleteObjectOutput, error) {
		if _, ok := objMap[*in.Key]; !ok {
			return nil, &types.NoSuchKey{}
		}
		// wasDeleted, shouldDelete := toDelete[*in.Key]
		// if wasDeleted {
		// 	t.Error("key already deleted", *in.Key)
		// }
		// if !shouldDelete {
		// 	t.Error("deleting key that shouldn't be deleted", *in.Key)
		// }
		out := &s3v2.DeleteObjectOutput{}
		return out, nil

	}
	return &S3API{List: listObjs, Delete: delete}
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

func objectMap(objs []*Object) map[string]*Object {
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
	return m.Upload(ctx, param, opts...)
}
func (m *S3API) UploadPartCopy(ctx context.Context, param *s3v2.UploadPartCopyInput, opts ...func(*s3v2.Options)) (*s3v2.UploadPartCopyOutput, error) {
	return m.UploadCopy(ctx, param, opts...)
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
