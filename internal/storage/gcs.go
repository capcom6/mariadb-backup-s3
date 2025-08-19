package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"cloud.google.com/go/storage"
)

type gcsStorage struct {
	bucket string
	prefix string
	client *storage.Client
}

func NewGCSStorage(u *url.URL) (StorageBackend, error) {
	// Get project ID from query parameters
	projectID := u.Query().Get("project")
	if projectID == "" {
		return nil, fmt.Errorf("GCS storage requires project ID in query parameters (project=your-project-id)")
	}

	// Create GCS client
	client, err := storage.NewClient(context.Background())
	if err != nil {
		return nil, fmt.Errorf("failed to create GCS client: %w", err)
	}

	prefix := strings.TrimPrefix(u.Path, "/")
	if prefix != "" && !strings.HasSuffix(prefix, "/") {
		prefix += "/"
	}

	return &gcsStorage{
		bucket: u.Host,
		prefix: prefix,
		client: client,
	}, nil
}

func (g *gcsStorage) Upload(ctx context.Context, path string, data io.Reader) error {
	key := g.prefix + path

	// Get bucket handle
	bucket := g.client.Bucket(g.bucket)
	obj := bucket.Object(key)

	// Create writer
	writer := obj.NewWriter(ctx)
	defer writer.Close()

	// Set content type
	writer.ContentType = "application/x-gzip"

	// Copy data
	if _, err := io.Copy(writer, data); err != nil {
		return fmt.Errorf("failed to upload to GCS: %w", err)
	}

	return nil
}

func (g *gcsStorage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}

	// Get bucket handle
	bucket := g.client.Bucket(g.bucket)

	// List objects with the prefix
	query := &storage.Query{Prefix: g.prefix}
	objects := bucket.Objects(ctx, query)

	// Collect all objects
	var objInfos []*storage.ObjectAttrs
	for {
		objAttrs, err := objects.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("failed to list objects: %w", err)
		}
		objInfos = append(objInfos, objAttrs)
	}

	if len(objInfos) <= maxCount {
		return nil
	}

	// Sort objects by name (which includes timestamp) to delete oldest
	sort.Slice(objInfos, func(i, j int) bool {
		return objInfos[i].Name < objInfos[j].Name
	})

	// Delete oldest objects
	toDelete := objInfos[:len(objInfos)-maxCount]
	for _, obj := range toDelete {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		if err := bucket.Object(obj.Name).Delete(ctx); err != nil {
			return fmt.Errorf("failed to delete object %s: %w", obj.Name, err)
		}
	}

	return nil
}
