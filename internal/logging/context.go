package logging

import (
	"context"
	"fmt"
	"maps"
	"time"
)

type contextKey string

const (
	ContextKeyInstance contextKey = "logging_instance"

	contextKeyComponent   contextKey = "logging_component"    // contextKeyComponent is the context key for component name
	contextKeyOperationID contextKey = "logging_operation_id" // contextKeyOperationID is the context key for operation ID
	contextKeyFields      contextKey = "logging_fields"       // contextKeyFields is the context key for additional fields
)

func WithLogger(ctx context.Context, logger Logger) context.Context {
	return context.WithValue(ctx, ContextKeyInstance, logger)
}

func GetLogger(ctx context.Context) Logger {
	if logger, ok := ctx.Value(ContextKeyInstance).(Logger); ok {
		return logger
	}
	return nil
}

// WithComponent adds a component name to the context.
func WithComponent(ctx context.Context, component string) context.Context {
	return context.WithValue(ctx, contextKeyComponent, component)
}

// getComponent retrieves the component name from context.
func getComponent(ctx context.Context) string {
	if component, ok := ctx.Value(contextKeyComponent).(string); ok {
		return component
	}
	return ""
}

// WithOperationID adds an operation ID to the context.
func WithOperationID(ctx context.Context, operationID string) context.Context {
	return context.WithValue(ctx, contextKeyOperationID, operationID)
}

// getOperationID retrieves the operation ID from context.
func getOperationID(ctx context.Context) string {
	if operationID, ok := ctx.Value(contextKeyOperationID).(string); ok {
		return operationID
	}
	return ""
}

// WithFields merges additional fields into context.
func WithFields(ctx context.Context, fields Fields) context.Context {
	if len(fields) == 0 {
		return ctx
	}
	current := getFields(ctx)
	merged := make(Fields, len(current)+len(fields))
	maps.Copy(merged, current)
	maps.Copy(merged, fields)
	return context.WithValue(ctx, contextKeyFields, merged)
}

// getFields retrieves the additional fields from context.
func getFields(ctx context.Context) Fields {
	if fields, ok := ctx.Value(contextKeyFields).(Fields); ok {
		return fields
	}
	return make(Fields)
}

// GenerateOperationID generates a simple operation ID based on component and current time.
func GenerateOperationID(component string) string {
	if component == "" {
		component = "unknown"
	}
	// Use a simple format: component-timestamp
	return fmt.Sprintf("%s-%d", component, currentTimeMillis())
}

// currentTimeMillis returns current time in milliseconds since epoch.
func currentTimeMillis() int64 {
	return time.Now().UnixMilli()
}
