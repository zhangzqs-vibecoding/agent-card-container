package publish

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type S3Config struct {
	Endpoint  string
	AccessKey string
	SecretKey string
	Bucket    string
	Region    string
	Secure    bool
}

type S3ObjectStore struct {
	client *minio.Client
	bucket string
	now    func() time.Time
}

func NewS3ObjectStore(ctx context.Context, config S3Config) (*S3ObjectStore, error) {
	if strings.TrimSpace(config.Endpoint) == "" ||
		strings.TrimSpace(config.AccessKey) == "" ||
		strings.TrimSpace(config.SecretKey) == "" ||
		strings.TrimSpace(config.Bucket) == "" {
		return nil, fmt.Errorf("S3 endpoint, credentials and bucket are required")
	}
	client, err := minio.New(config.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(config.AccessKey, config.SecretKey, ""),
		Secure: config.Secure,
		Region: config.Region,
	})
	if err != nil {
		return nil, err
	}
	exists, err := client.BucketExists(ctx, config.Bucket)
	if err != nil {
		return nil, err
	}
	if !exists {
		if err := client.MakeBucket(ctx, config.Bucket, minio.MakeBucketOptions{Region: config.Region}); err != nil {
			return nil, err
		}
	}
	return &S3ObjectStore{
		client: client,
		bucket: config.Bucket,
		now:    time.Now,
	}, nil
}

func (store *S3ObjectStore) PutIfAbsent(
	ctx context.Context,
	key string,
	content []byte,
) error {
	options := minio.PutObjectOptions{ContentType: "application/vnd.agentcard+zip"}
	options.SetMatchETagExcept("*")
	_, err := store.client.PutObject(
		ctx,
		store.bucket,
		key,
		bytes.NewReader(content),
		int64(len(content)),
		options,
	)
	if err == nil {
		return nil
	}
	response := minio.ToErrorResponse(err)
	if response.Code != "PreconditionFailed" && response.StatusCode != 412 {
		return err
	}
	info, statErr := store.client.StatObject(ctx, store.bucket, key, minio.StatObjectOptions{})
	if statErr != nil {
		return statErr
	}
	if info.Size != int64(len(content)) {
		return ErrObjectConflict
	}
	return nil
}

func (store *S3ObjectStore) SignedURL(
	ctx context.Context,
	key string,
	ttl time.Duration,
) (string, time.Time, error) {
	if ttl <= 0 {
		return "", time.Time{}, fmt.Errorf("signed URL TTL must be positive")
	}
	signed, err := store.client.PresignedGetObject(
		ctx,
		store.bucket,
		key,
		ttl,
		url.Values{},
	)
	if err != nil {
		return "", time.Time{}, err
	}
	return signed.String(), store.now().UTC().Add(ttl), nil
}
