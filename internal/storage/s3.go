package storage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/aws/smithy-go"
)

// S3Provider implements storage.Provider using AWS S3
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
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to generate S3 pre-signed upload URL: %w", err)
	}
	return req.URL, nil
}

func (s *S3Provider) GenerateDownloadURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	filename := filepath.Base(key)
	req, err := s.presignClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket:                     aws.String(s.bucket),
		Key:                        aws.String(key),
		ResponseContentDisposition: aws.String(fmt.Sprintf("inline; filename=%q", filename)),
	}, s3.WithPresignExpires(expiry))
	if err != nil {
		return "", fmt.Errorf("failed to generate S3 pre-signed download URL: %w", err)
	}
	return req.URL, nil
}

func (s *S3Provider) DeleteFile(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete S3 object: %w", err)
	}
	return nil
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
			code := apiErr.ErrorCode()
			if code == "NotFound" || code == "NoSuchKey" || code == "404" {
				return false, nil
			}
		}
		return false, fmt.Errorf("failed to verify S3 object existence: %w", err)
	}
	return true, nil
}

func (s *S3Provider) GetFileBytes(ctx context.Context, key string) ([]byte, string, error) {
	out, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &s.bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, "", err
	}
	defer out.Body.Close()

	if out.ContentLength != nil && *out.ContentLength > MaxOCRFileSize {
		return nil, "", fmt.Errorf("%w: size %d exceeds limit of %d bytes", ErrFileTooLarge, *out.ContentLength, MaxOCRFileSize)
	}

	data, err := io.ReadAll(io.LimitReader(out.Body, MaxOCRFileSize+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxOCRFileSize {
		return nil, "", fmt.Errorf("%w: file exceeds limit of %d bytes", ErrFileTooLarge, MaxOCRFileSize)
	}

	mimeType := ""
	if out.ContentType != nil {
		mimeType = strings.ToLower(strings.TrimSpace(strings.Split(*out.ContentType, ";")[0]))
	}
	if mimeType == "" || mimeType == "application/octet-stream" || mimeType == "binary/octet-stream" {
		mimeType = http.DetectContentType(data)
		mimeType = strings.ToLower(strings.TrimSpace(strings.Split(mimeType, ";")[0]))
	}
	return data, mimeType, nil
}
