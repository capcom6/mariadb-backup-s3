package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

type s3Storage struct {
	bucket   string
	prefix   string
	client   *s3.Client
	partSize int64
}

func NewS3Storage(u *url.URL) (Backend, error) {
	forcePathStyle := false
	var partSize int64 = 10 * 1024 * 1024 // Default 10 MB

	endpoint := u.Query().Get("endpoint")
	forcePathStyleRaw := u.Query().Get("s3-force-path-style")
	if forcePathStyleRaw != "" {
		var err error
		forcePathStyle, err = strconv.ParseBool(forcePathStyleRaw)
		if err != nil {
			return nil, fmt.Errorf("failed to parse s3-force-path-style: %w", err)
		}
	}

	partSizeRaw := u.Query().Get("part-size")
	if partSizeRaw != "" {
		var err error
		partSize, err = strconv.ParseInt(partSizeRaw, 10, 64)
		if err != nil {
			return nil, fmt.Errorf("failed to parse part-size: %w", err)
		}
		if partSize < 5*1024*1024 {
			return nil, fmt.Errorf("part-size must be at least 5MB (5242880 bytes)")
		}
	}

	sdkConfig, err := config.LoadDefaultConfig(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS SDK config: %w", err)
	}

	prefix := strings.TrimPrefix(u.Path, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	s3Options := []func(*s3.Options){}
	if endpoint != "" {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.BaseEndpoint = aws.String(endpoint)
		})
	}
	if forcePathStyle {
		s3Options = append(s3Options, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	return &s3Storage{
		bucket:   u.Host,
		prefix:   prefix,
		client:   s3.NewFromConfig(sdkConfig, s3Options...),
		partSize: partSize,
	}, nil
}

func (s *s3Storage) Upload(ctx context.Context, path string, data io.Reader) error {
	key := s.prefix + strings.TrimPrefix(path, "/")

	uploader := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = s.partSize
	})
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: aws.String("application/gzip"),
		Body:        data,
	})
	if err != nil {
		return fmt.Errorf("failed to upload to S3: %w", err)
	}

	return nil
}

func (s *s3Storage) Download(ctx context.Context, path string, data io.Writer) error {
	key := s.prefix + strings.TrimPrefix(path, "/")

	// Use GetObject directly since downloader requires WriterAt
	getObjectOutput, err := s.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to get object from S3: %w", err)
	}
	defer func() {
		_ = getObjectOutput.Body.Close()
	}()

	_, err = io.Copy(data, getObjectOutput.Body)
	if err != nil {
		return fmt.Errorf("failed to copy S3 object data: %w", err)
	}

	return nil
}

func (s *s3Storage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}
	if maxCount < 0 {
		return fmt.Errorf("%w: maxCount must be >= 0", ErrInvalidArgument)
	}

	var err error
	var output *s3.ListObjectsV2Output
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix),
	}
	objects := make([]types.ObjectIdentifier, 0, maxCount+1)
	objectPaginator := s3.NewListObjectsV2Paginator(s.client, input)
	for objectPaginator.HasMorePages() {
		output, err = objectPaginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		for _, obj := range output.Contents {
			objects = append(objects, types.ObjectIdentifier{
				Key: obj.Key,
			})
		}
	}

	if len(objects) <= maxCount {
		return nil
	}

	sort.Slice(objects, func(i, j int) bool {
		return aws.ToString(objects[i].Key) < aws.ToString(objects[j].Key)
	})

	toDelete := objects[:len(objects)-maxCount]

	_, err = s.client.DeleteObjects(ctx, &s3.DeleteObjectsInput{
		Bucket: aws.String(s.bucket),
		Delete: &types.Delete{
			Objects: toDelete,
			Quiet:   aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("failed to delete objects: %w", err)
	}

	return nil
}
