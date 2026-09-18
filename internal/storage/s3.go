package storage

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Provider implements storage.Provider for AWS S3 and S3-compatible object stores
type S3Provider struct {
	client        *s3.Client
	presignClient *s3.PresignClient
	bucket        string
}

// NewS3Provider creates a new S3Provider instance
func NewS3Provider(client *s3.Client, bucket string) *S3Provider {
	return &S3Provider{
		client:        client,
		presignClient: s3.NewPresignClient(client),
		bucket:        bucket,
	}
}

func (s *S3Provider) GenerateUploadURL(ctx context.Context, key string, contentType string, expiry time.Duration) (string, error) {
	req, err := s.presignClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String(contentType),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Provider) GenerateDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	filename := filepath.Base(key)
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(fmt.Sprintf("attachment; filename=%q", filename)),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expiry
	})
	if err != nil {
		return "", err
	}
	return req.URL, nil
}

func (s *S3Provider) DeleteFile(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	return err
}

func (s *S3Provider) ObjectExists(ctx context.Context, key string) (bool, error) {
	_, err := s.client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		var notFound *types.NotFound
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &notFound) || errors.As(err, &noSuchKey) {
			return false, nil
		}
		var apiErr smithy.APIError
		if errors.As(err, &apiErr) {
			if apiErr.ErrorCode() == "NotFound" || apiErr.ErrorCode() == "NoSuchKey" {
				return false, nil
			}
		}
		if strings.Contains(err.Error(), "404") || strings.Contains(err.Error(), "NotFound") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
