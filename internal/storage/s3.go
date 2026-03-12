package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/url"
	"path"
	"strconv"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/feature/s3/manager"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/aws/aws-sdk-go-v2/service/s3/types"
)

const (
	// S3DefaultPartSize is the default part size for S3 multipart uploads (10 MB).
	S3DefaultPartSize int64 = 10 * 1024 * 1024
	// S3MinPartSize is the minimum part size for S3 multipart uploads (5 MB).
	S3MinPartSize int64 = 5 * 1024 * 1024
	// S3MaxPartSize is the maximum part size for S3 multipart uploads (5 GiB).
	S3MaxPartSize int64 = 5 * 1024 * 1024 * 1024
)

type s3Storage struct {
	bucket   string
	prefix   string
	partSize int64

	client *s3.Client
}

func NewS3Storage(u *url.URL) (Backend, error) {
	forcePathStyle := false
	partSize := S3DefaultPartSize

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
		if partSize < S3MinPartSize {
			return nil, fmt.Errorf("%w: part-size must be at least %d bytes", ErrInvalidArgument, S3MinPartSize)
		}
		if partSize > S3MaxPartSize {
			return nil, fmt.Errorf("%w: part-size must be at most %d bytes", ErrInvalidArgument, S3MaxPartSize)
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
		partSize: partSize,

		client: s3.NewFromConfig(sdkConfig, s3Options...),
	}, nil
}

func (s *s3Storage) Upload(ctx context.Context, p string, data io.Reader) error {
	key := s.prefix + strings.TrimPrefix(p, "/")

	extension := path.Ext(p)
	contentType := mime.TypeByExtension(extension)
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	uploader := manager.NewUploader(s.client, func(u *manager.Uploader) {
		u.PartSize = s.partSize
	})
	_, err := uploader.Upload(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(key),
		ContentType: &contentType,
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
		var noSuchKey *types.NoSuchKey
		if errors.As(err, &noSuchKey) {
			return fmt.Errorf("failed to get object from S3: %w", ErrNotFound)
		}
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

func (s *s3Storage) UploadBytes(ctx context.Context, path string, data []byte) error {
	return s.Upload(ctx, path, bytes.NewReader(data))
}

func (s *s3Storage) DownloadBytes(ctx context.Context, path string) ([]byte, error) {
	var buf bytes.Buffer
	if err := s.Download(ctx, path, &buf); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

func (s *s3Storage) Delete(ctx context.Context, path string) error {
	key := s.prefix + strings.TrimPrefix(path, "/")
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return fmt.Errorf("failed to delete object: %w", err)
	}

	return nil
}

func (s *s3Storage) List(ctx context.Context) ([]string, error) {
	input := &s3.ListObjectsV2Input{
		Bucket: aws.String(s.bucket),
		Prefix: aws.String(s.prefix),
	}

	files := make([]string, 0)
	p := s3.NewListObjectsV2Paginator(s.client, input)
	for p.HasMorePages() {
		out, err := p.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to list objects: %w", err)
		}
		for _, obj := range out.Contents {
			key := aws.ToString(obj.Key)
			files = append(files, strings.TrimPrefix(key, s.prefix))
		}
	}

	return files, nil
}

// Close closes the S3 storage backend.
// For S3, this is a no-op as the AWS SDK client doesn't require explicit cleanup.
func (s *s3Storage) Close() error {
	return nil
}
