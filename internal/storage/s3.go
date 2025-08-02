package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3"
)

type s3Storage struct {
	bucket string
	prefix string
	svc    *s3.S3
}

func NewS3Storage(u *url.URL) (StorageBackend, error) {
	endpoint := u.Query().Get("endpoint")
	forcePathStyle := u.Query().Get("s3-force-path-style") == "true"

	sess, err := session.NewSession(&aws.Config{
		Endpoint:         aws.String(endpoint),
		S3ForcePathStyle: aws.Bool(forcePathStyle),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create AWS session: %w", err)
	}

	prefix := strings.TrimPrefix(u.Path, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &s3Storage{
		bucket: u.Host,
		prefix: prefix,
		svc:    s3.New(sess),
	}, nil
}

func (s *s3Storage) Upload(ctx context.Context, path string, data io.Reader) error {
	key := s.prefix + path

	_, err := s.svc.PutObjectWithContext(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/x-gzip"),
		Body:        aws.ReadSeekCloser(data),
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

func (s *s3Storage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}

	keys := make([]*s3.ObjectIdentifier, 0, maxCount+1)

	err := s.svc.ListObjectsV2PagesWithContext(ctx, &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix),
	}, func(p *s3.ListObjectsV2Output, last bool) bool {
		for _, obj := range p.Contents {
			keys = append(keys, &s3.ObjectIdentifier{Key: obj.Key})
		}
		return true
	})
	if err != nil {
		return fmt.Errorf("failed to list objects: %w", err)
	}

	if len(keys) <= maxCount {
		return nil
	}

	// Sort keys by name (which includes timestamp) to delete oldest
	sort.Slice(keys, func(i, j int) bool {
		return *keys[i].Key < *keys[j].Key
	})

	toDelete := keys[:len(keys)-maxCount]

	_, err = s.svc.DeleteObjectsWithContext(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &s3.Delete{
			Objects: toDelete,
			Quiet:   aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}

	return nil
}
