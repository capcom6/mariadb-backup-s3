package storage

import (
	"context"
	"fmt"
	"io"
	"net/url"
	"sort"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
)

type azureStorage struct {
	accountName   string
	containerName string
	prefix        string
	client        *azblob.Client
}

func NewAzureStorage(u *url.URL) (StorageBackend, error) {
	// Get account name from host
	accountName := u.Host
	if accountName == "" {
		return nil, fmt.Errorf("Azure storage requires account name in URL host")
	}

	// Get container name from path
	containerName := strings.TrimPrefix(u.Path, "/")
	if containerName == "" {
		return nil, fmt.Errorf("Azure storage requires container name in URL path")
	}

	// Get SAS token from query parameters
	sasToken := u.Query().Get("sas")

	// Create Azure Blob Storage client
	var client *azblob.Client
	var err error

	if sasToken != "" {
		// Use connection string with SAS token
		connStr := fmt.Sprintf("DefaultEndpointsProtocol=https;AccountName=%s;AccountKey=%s;EndpointSuffix=core.windows.net", accountName, sasToken)
		client, err = azblob.NewClientFromConnectionString(connStr, nil)
	} else {
		// Use anonymous client (for public containers)
		client, err = azblob.NewClient(fmt.Sprintf("https://%s.blob.core.windows.net/", accountName), nil, nil)
	}

	if err != nil {
		return nil, fmt.Errorf("failed to create Azure client: %w", err)
	}

	prefix := ""
	if len(u.Path) > len("/"+containerName) {
		prefix = strings.TrimPrefix(u.Path[len("/"+containerName):], "/")
		if prefix != "" && !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
	}

	return &azureStorage{
		accountName:   accountName,
		containerName: containerName,
		prefix:        prefix,
		client:        client,
	}, nil
}

func (a *azureStorage) Upload(ctx context.Context, path string, data io.Reader) error {
	blobName := a.prefix + path

	// Upload blob
	_, err := a.client.UploadBuffer(ctx, a.containerName, blobName, []byte{}, nil)
	if err != nil {
		return fmt.Errorf("failed to upload to Azure: %w", err)
	}

	return nil
}

func (a *azureStorage) DeleteOldBackups(ctx context.Context, maxCount int) error {
	if maxCount == 0 {
		return nil
	}

	// List blobs with the prefix
	pager := a.client.NewListBlobsFlatPager(a.containerName, &azblob.ListBlobsFlatOptions{
		Prefix: &a.prefix,
	})

	// Collect all blobs
	var blobNames []string
	for pager.More() {
		resp, err := pager.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list blobs: %w", err)
		}

		for _, blob := range resp.Segment.BlobItems {
			blobNames = append(blobNames, *blob.Name)
		}
	}

	if len(blobNames) <= maxCount {
		return nil
	}

	// Sort blob names by name (which includes timestamp) to delete oldest
	sort.Strings(blobNames)

	// Delete oldest blobs
	toDelete := blobNames[:len(blobNames)-maxCount]
	for _, blobName := range toDelete {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		_, err := a.client.DeleteBlob(ctx, a.containerName, blobName, nil)
		if err != nil {
			return fmt.Errorf("failed to delete blob %s: %w", blobName, err)
		}
	}

	return nil
}

// Helper function to convert string to pointer
func ptrToString(s string) *string {
	return &s
}
