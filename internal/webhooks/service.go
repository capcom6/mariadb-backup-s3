package webhooks

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/client"
	eventshttp "github.com/cloudevents/sdk-go/v2/protocol/http"
)

const (
	eventSource = "com.capcom6.mariadb-backup-s3"
	eventType   = "com.capcom6.mariadb-backup-s3.scheduler.job.completed"
)

type Service struct {
	config Config

	client client.Client
}

// NewService builds the webhook service. An empty (or whitespace-only)
// DefaultURL yields a disabled service: no client, nil error, and Notify is a
// no-op. A configured URL keeps the existing client construction behavior.
func NewService(config Config) (*Service, error) {
	if strings.TrimSpace(config.DefaultURL) == "" {
		return &Service{config: config, client: nil}, nil
	}

	client, err := cloudevents.NewClientHTTP(
		cloudevents.WithTarget(config.DefaultURL),
		eventshttp.WithClient(http.Client{
			Timeout: config.DefaultTimeout,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Service{
		config: config,

		client: client,
	}, nil
}

func (s *Service) Notify(ctx context.Context, payload Payload) error {
	if s.client == nil {
		return nil
	}

	event := cloudevents.NewEvent()
	event.SetSource(eventSource)
	event.SetType(eventType)
	if err := event.SetData(cloudevents.ApplicationJSON, payload); err != nil {
		return fmt.Errorf("failed to set data: %w", err)
	}

	// Set a target.
	ctx = cloudevents.ContextWithTarget(ctx, s.config.DefaultURL)

	if result := s.client.Send(ctx, event); cloudevents.IsUndelivered(result) || cloudevents.IsNACK(result) {
		return fmt.Errorf("failed to send event: %w", result)
	}

	return nil
}
