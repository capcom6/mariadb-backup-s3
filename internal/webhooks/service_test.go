package webhooks_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/capcom6/mariadb-backup-s3/internal/webhooks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// These tests intentionally do not call t.Parallel: TestNewService_EmptyURL_...
// swaps http.DefaultTransport to prove no HTTP request happens, which is only
// race-free when the package's tests run sequentially.

type recordingTransport struct {
	base  http.RoundTripper
	count *int32
}

func (t recordingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	atomic.AddInt32(t.count, 1)
	return t.base.RoundTrip(r)
}

func TestNewService_EmptyURL_DisabledWithoutHTTPRequest(t *testing.T) {
	prevTransport := http.DefaultTransport
	var count int32
	//nolint:reassign // intercept the default transport to prove zero HTTP requests leave the test process
	http.DefaultTransport = recordingTransport{base: prevTransport, count: &count}
	t.Cleanup(func() {
		//nolint:reassign // restore the transport captured before the swap above
		http.DefaultTransport = prevTransport
	})

	for _, url := range []string{"", "   ", "\t\n "} {
		svc, err := webhooks.NewService(webhooks.Config{
			DefaultURL:     url,
			DefaultTimeout: time.Second,
		})
		require.NoError(t, err, "url=%q", url)
		require.NotNil(t, svc, "url=%q", url)

		notifyErr := svc.Notify(context.Background(), webhooks.Payload{
			Event:   "job.completed",
			JobName: "mongo-backup",
			Status:  "success",
		})
		require.NoError(t, notifyErr, "url=%q", url)
	}

	assert.Zero(t, atomic.LoadInt32(&count), "disabled service must not perform HTTP requests")
}

func TestNewService_ConfiguredURL_NotifyDeliversCloudEvent(t *testing.T) {
	var (
		mu  sync.Mutex
		got []capturedRequest
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, readErr := io.ReadAll(r.Body)
		mu.Lock()
		got = append(got, capturedRequest{
			method:  r.Method,
			path:    r.URL.Path,
			header:  r.Header.Clone(),
			body:    body,
			readErr: readErr,
		})
		mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	svc, err := webhooks.NewService(webhooks.Config{
		DefaultURL:     srv.URL,
		DefaultTimeout: 5 * time.Second,
	})
	require.NoError(t, err)

	notifyErr := svc.Notify(context.Background(), webhooks.Payload{
		Event:   "job.completed",
		JobName: "mongo-backup",
		Status:  "success",
		Error:   "",
	})
	require.NoError(t, notifyErr)

	mu.Lock()
	defer mu.Unlock()
	require.Len(t, got, 1)
	require.NoError(t, got[0].readErr)

	assert.Equal(t, http.MethodPost, got[0].method)
	assert.Equal(t, "/", got[0].path)

	eventType, source, data := cloudEventAttrs(t, got[0].header, got[0].body)
	assert.Equal(t, "com.capcom6.mariadb-backup-s3.scheduler.job.completed", eventType)
	assert.Equal(t, "com.capcom6.mariadb-backup-s3", source)
	require.Contains(t, data, "event")
	assert.Equal(t, "job.completed", data["event"])
	assert.Equal(t, "mongo-backup", data["job_name"])
	assert.Equal(t, "success", data["status"])
}

func TestNewService_UnreachableURL_NotifyReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	unreachableURL := srv.URL
	srv.Close()

	svc, err := webhooks.NewService(webhooks.Config{
		DefaultURL:     unreachableURL,
		DefaultTimeout: 3 * time.Second,
	})
	require.NoError(t, err, "non-empty URL keeps existing client construction behavior")

	notifyErr := svc.Notify(context.Background(), webhooks.Payload{
		Event:   "job.completed",
		JobName: "mongo-backup",
		Status:  "failed",
		Error:   "boom",
	})
	require.Error(t, notifyErr)
	assert.Contains(t, notifyErr.Error(), "failed to send event")
}

type capturedRequest struct {
	method  string
	path    string
	header  http.Header
	body    []byte
	readErr error
}

// cloudEventAttrs extracts type, source and data from either binary-mode
// headers or a structured-mode application/cloudevents+json envelope.
func cloudEventAttrs(t *testing.T, header http.Header, body []byte) (string, string, map[string]any) {
	t.Helper()

	contentType := header.Get("Content-Type")
	if strings.HasPrefix(contentType, "application/cloudevents+json") {
		var envelope struct {
			Type   string         `json:"type"`
			Source string         `json:"source"`
			Data   map[string]any `json:"data"`
		}
		require.NoError(t, json.Unmarshal(body, &envelope))
		return envelope.Type, envelope.Source, envelope.Data
	}

	var data map[string]any
	require.NoError(t, json.Unmarshal(body, &data))
	return header.Get("Ce-Type"), header.Get("Ce-Source"), data
}
