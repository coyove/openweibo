package kv

import (
	"io"
	"net/http"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
	//sync "github.com/sasha-s/go-deadlock"
)

type S3Storage struct {
	db     *s3manager.Uploader
	bucket string
}

func NewS3Storage(endpoint, region, bucket, accessKey, secretKey string) *S3Storage {
	sess, err := session.NewSession(&aws.Config{
		Endpoint:    aws.String(endpoint),
		Region:      aws.String(region),
		Credentials: credentials.NewStaticCredentials(accessKey, secretKey, ""),
		HTTPClient: &http.Client{
			Timeout: time.Second * 10,
			Transport: &http.Transport{
				MaxConnsPerHost: 200,
			},
		},
	})
	if err != nil {
		panic(err)
	}
	db := s3manager.NewUploader(sess)
	r := &S3Storage{
		db:     db,
		bucket: bucket,
	}
	return r
}

func (m *S3Storage) Put(key string, contentType string, file io.Reader) error {
	_, err := m.db.Upload(&s3manager.UploadInput{
		Bucket:      aws.String(m.bucket),
		ContentType: aws.String(contentType),
		Key:         aws.String(key),
		Body:        file,
	})
	return err
}
