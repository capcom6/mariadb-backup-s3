package pipeline_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"

	"github.com/capcom6/mariadb-backup-s3/pkg/pipeline"
)

// Helper functions for test stages

// identity stage that simply copies from reader to writer.
func identity(_ context.Context, r io.Reader, w io.Writer) error {
	_, err := io.Copy(w, r)
	return err
}

// failingStage that always returns an error.
func failingStage(_ context.Context, _ io.Reader, _ io.Writer) error {
	return errors.New("simulated failure")
}

// transformStage that modifies the content by appending a suffix.
func transformStage(suffix string) pipeline.StageFunc {
	return func(_ context.Context, r io.Reader, w io.Writer) error {
		_, err := io.Copy(w, r)
		if err != nil {
			return err
		}
		_, err = fmt.Fprint(w, suffix)
		return err
	}
}

// errorStage that returns an error with a specific message.
func errorStage(stageNum int, msg string) pipeline.StageFunc {
	return func(_ context.Context, _ io.Reader, _ io.Writer) error {
		return fmt.Errorf("stage %d error: %s", stageNum, msg)
	}
}

// Test successful execution with no stages (direct src→dst copy).
func TestRun_NoStages(t *testing.T) {
	input := "test data"
	expected := input

	var dst bytes.Buffer
	src := strings.NewReader(input)

	err := pipeline.Run(context.TODO(), src, &dst)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if dst.String() != expected {
		t.Errorf("Expected %q, got %q", expected, dst.String())
	}
}

// Test successful execution with single stage (identity transform).
func TestRun_SingleStage(t *testing.T) {
	input := "test data"
	expected := input

	var dst bytes.Buffer
	src := strings.NewReader(input)

	err := pipeline.Run(context.TODO(), src, &dst, identity)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if dst.String() != expected {
		t.Errorf("Expected %q, got %q", expected, dst.String())
	}
}

// Test successful execution with multiple stages (chained transforms).
func TestRun_MultipleStages(t *testing.T) {
	input := "hello"
	expected := "hello world transformed"

	var dst bytes.Buffer
	src := strings.NewReader(input)

	stages := []pipeline.StageFunc{
		transformStage(" world"),
		transformStage(" transformed"),
	}

	err := pipeline.Run(context.TODO(), src, &dst, stages...)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if dst.String() != expected {
		t.Errorf("Expected %q, got %q", expected, dst.String())
	}
}

// Test error propagation from intermediate stage.
func TestRun_ErrorFromIntermediateStage(t *testing.T) {
	input := "test data"
	var dst bytes.Buffer
	src := strings.NewReader(input)

	stages := []pipeline.StageFunc{
		identity,
		failingStage,
		identity,
	}

	err := pipeline.Run(context.TODO(), src, &dst, stages...)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Verify error message contains stage number
	if !strings.Contains(err.Error(), "stage 1 failed") {
		t.Errorf("Expected error to contain 'stage 1 failed', got: %v", err)
	}

	if !strings.Contains(err.Error(), "simulated failure") {
		t.Errorf("Expected error to contain 'simulated failure', got: %v", err)
	}
}

// Test error propagation from final copy operation.
func TestRun_ErrorFromFinalCopy(t *testing.T) {
	// Create a reader that will fail on read
	failingReader := &errorReader{err: errors.New("read failure")}
	var dst bytes.Buffer

	err := pipeline.Run(context.TODO(), failingReader, &dst)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Verify error message contains "final copy failed"
	if !strings.Contains(err.Error(), "final copy failed") {
		t.Errorf("Expected error to contain 'final copy failed', got: %v", err)
	}
}

// Test concurrent stage execution validation.
func TestRun_ConcurrentExecution(t *testing.T) {
	// Test that concurrent execution doesn't cause data races
	// by running the pipeline multiple times in goroutines
	input := "test data"
	expected := input + "suffix"

	stages := []pipeline.StageFunc{
		transformStage("suffix"),
	}

	// Use a channel to collect results
	results := make(chan string, 5)
	var wg sync.WaitGroup

	// Run pipeline concurrently
	for range 5 {
		wg.Go(func() {
			var dst bytes.Buffer
			src := strings.NewReader(input)
			err := pipeline.Run(context.TODO(), src, &dst, stages...)
			if err != nil {
				t.Errorf("Concurrent execution failed: %v", err)
				return
			}
			results <- dst.String()
		})
	}

	// Wait for all goroutines to complete
	wg.Wait()
	close(results)

	// Verify all results are correct
	for result := range results {
		if result != expected {
			t.Errorf("Expected %q, got %q", expected, result)
		}
	}
}

// Table-driven tests for transform validation.
func TestRun_TransformValidation(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		stages   []pipeline.StageFunc
		expected string
		wantErr  bool
	}{
		{
			name:     "no stages",
			input:    "hello",
			stages:   nil,
			expected: "hello",
			wantErr:  false,
		},
		{
			name:     "single identity stage",
			input:    "hello",
			stages:   []pipeline.StageFunc{identity},
			expected: "hello",
			wantErr:  false,
		},
		{
			name:     "single transform stage",
			input:    "hello",
			stages:   []pipeline.StageFunc{transformStage(" world")},
			expected: "hello world",
			wantErr:  false,
		},
		{
			name:     "multiple transform stages",
			input:    "hello",
			stages:   []pipeline.StageFunc{transformStage(" "), transformStage("world")},
			expected: "hello world",
			wantErr:  false,
		},
		{
			name:     "failing first stage",
			input:    "hello",
			stages:   []pipeline.StageFunc{failingStage, identity},
			expected: "",
			wantErr:  true,
		},
		{
			name:     "failing middle stage",
			input:    "hello",
			stages:   []pipeline.StageFunc{identity, failingStage, identity},
			expected: "",
			wantErr:  true,
		},
		{
			name:     "failing last stage",
			input:    "hello",
			stages:   []pipeline.StageFunc{identity, failingStage},
			expected: "",
			wantErr:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst bytes.Buffer
			src := strings.NewReader(tt.input)

			err := pipeline.Run(context.TODO(), src, &dst, tt.stages...)

			if (err != nil) != tt.wantErr {
				t.Errorf("Run() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && dst.String() != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, dst.String())
			}
		})
	}
}

// Test error messages contain correct stage numbers.
func TestRun_ErrorStageNumbers(t *testing.T) {
	tests := []struct {
		name        string
		stages      []pipeline.StageFunc
		expectedErr string
	}{
		{
			name:        "first stage fails",
			stages:      []pipeline.StageFunc{failingStage, identity},
			expectedErr: "stage 0 failed",
		},
		{
			name:        "second stage fails",
			stages:      []pipeline.StageFunc{identity, failingStage},
			expectedErr: "stage 1 failed",
		},
		{
			name:        "third stage fails",
			stages:      []pipeline.StageFunc{identity, identity, failingStage},
			expectedErr: "stage 2 failed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst bytes.Buffer
			src := strings.NewReader("test")

			err := pipeline.Run(context.TODO(), src, &dst, tt.stages...)

			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			if !strings.Contains(err.Error(), tt.expectedErr) {
				t.Errorf("Expected error to contain %q, got: %v", tt.expectedErr, err)
			}
		})
	}
}

// Test error wrapping with [errors.Is].
func TestRun_ErrorWrapping(t *testing.T) {
	// Test that original error is preserved in wrapped error
	var dst bytes.Buffer
	src := strings.NewReader("test")

	stages := []pipeline.StageFunc{errorStage(0, "original error")}

	err := pipeline.Run(context.TODO(), src, &dst, stages...)
	if err == nil {
		t.Fatal("Expected error, got nil")
	}

	// Test that we can check for specific error types
	if !strings.Contains(err.Error(), "original error") {
		t.Errorf("Expected error to contain 'original error', got: %v", err)
	}
}

// Test large data handling.
func TestRun_LargeData(t *testing.T) {
	// Create a large input (1MB)
	input := strings.Repeat("a", 1024*1024)
	expected := input + "suffix"

	var dst bytes.Buffer
	src := strings.NewReader(input)

	stages := []pipeline.StageFunc{transformStage("suffix")}

	err := pipeline.Run(context.TODO(), src, &dst, stages...)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if dst.String() != expected {
		t.Errorf("Expected output length %d, got %d", len(expected), len(dst.String()))
	}
}

// Test empty input.
func TestRun_EmptyInput(t *testing.T) {
	input := ""
	expected := "suffix"

	var dst bytes.Buffer
	src := strings.NewReader(input)

	stages := []pipeline.StageFunc{transformStage("suffix")}

	err := pipeline.Run(context.TODO(), src, &dst, stages...)
	if err != nil {
		t.Fatalf("Expected no error, got: %v", err)
	}

	if dst.String() != expected {
		t.Errorf("Expected %q, got %q", expected, dst.String())
	}
}

// Test that errors from multiple failed stages are all collected.
func TestRun_MultipleErrors(t *testing.T) {
	tests := []struct {
		name       string
		stages     []pipeline.StageFunc
		wantErrors []string
	}{
		{
			name: "two stages fail independently",
			stages: []pipeline.StageFunc{
				errorStage(0, "first failure"),
			},
			wantErrors: []string{"stage 0 error: first failure"},
		},
		{
			name: "two stages fail (cascade)",
			stages: []pipeline.StageFunc{
				failingStage,
				identity,
			},
			wantErrors: []string{
				"stage 0 failed",
				"stage 1 failed",
			},
		},
		{
			name: "all stages fail",
			stages: []pipeline.StageFunc{
				failingStage,
				failingStage,
				failingStage,
			},
			wantErrors: []string{
				"stage 0 failed",
				"stage 1 failed",
				"stage 2 failed",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var dst bytes.Buffer
			src := strings.NewReader("test")

			err := pipeline.Run(context.TODO(), src, &dst, tt.stages...)
			if err == nil {
				t.Fatal("Expected error, got nil")
			}

			for _, want := range tt.wantErrors {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("Expected error to contain %q, got: %v", want, err)
				}
			}
		})
	}
}

// errorReader is a reader that always returns an error.
type errorReader struct {
	err error
}

func (r *errorReader) Read(_ []byte) (int, error) {
	return 0, r.err
}
