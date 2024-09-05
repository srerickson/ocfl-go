package testutil

import (
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

func S3Client() *s3.Client {
	return s3.NewFromConfig(aws.Config{Region: "us-east-1"}, func(o *s3.Options) {
		o.BaseEndpoint = aws.String("http://127.0.0.1:9000")
		o.Credentials = credentials.NewStaticCredentialsProvider("minioadmin", "minioadmin", "")
	})
}

// func removeBucket(s3cl *s3.S3, name string) error {
// 	b := aws.String(name)
// 	var listFuncErr error
// 	listopts := &s3.ListObjectsV2Input{Bucket: b}
// 	listFunc := func(out *s3.ListObjectsV2Output, last bool) bool {
// 		for _, obj := range out.Contents {
// 			if _, err := s3cl.DeleteObject(&s3.DeleteObjectInput{
// 				Bucket: b,
// 				Key:    obj.Key,
// 			}); err != nil {
// 				listFuncErr = fmt.Errorf("removing %q: %w", *obj.Key, err)
// 				return false
// 			}
// 		}
// 		return !last
// 	}
// 	if err := s3cl.ListObjectsV2Pages(listopts, listFunc); err != nil {
// 		return err
// 	}
// 	if listFuncErr != nil {
// 		return listFuncErr
// 	}
// 	_, err := s3cl.DeleteBucket(&s3.DeleteBucketInput{
// 		Bucket: aws.String(name),
// 	})
// 	return err
// }
