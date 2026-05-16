package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIODownloader struct {
	client *minio.Client
	bucket string
}

func NewMinIODownloader(endpoint, accessKey, secretKey, bucket string, useSSL bool) (*MinIODownloader, error) {
	client, err := minio.New(endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(accessKey, secretKey, ""),
		Secure: useSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("minio client init: %w", err)
	}
	return &MinIODownloader{client: client, bucket: bucket}, nil
}

func (d *MinIODownloader) Download(ctx context.Context, key string) ([]byte, string, error) {
	obj, err := d.client.GetObject(ctx, d.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, "", fmt.Errorf("minio get object: %w", err)
	}
	defer obj.Close()

	stat, err := obj.Stat()
	if err != nil {
		return nil, "", fmt.Errorf("minio stat object: %w", err)
	}

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, "", fmt.Errorf("minio read object: %w", err)
	}

	return data, stat.ContentType, nil
}
