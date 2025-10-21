package pipeline

import (
	"context"
	"fmt"
	"io"
	"sync"
)

// StageFunc defines a processing step that can be canceled via context.
type StageFunc func(context.Context, io.Reader, io.Writer) error

// Run runs the pipeline with context for cancellation.
func Run(ctx context.Context, src io.Reader, dst io.Writer, stages ...StageFunc) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	var wg sync.WaitGroup
	errCh := make(chan error, len(stages)+1)

	in := src

	for i, stage := range stages {
		r, w := io.Pipe()
		wg.Add(1)
		go func(stage StageFunc, in io.Reader, w *io.PipeWriter, i int) {
			defer wg.Done()

			var closeOnce sync.Once
			closeWith := func(err error) {
				closeOnce.Do(func() { _ = w.CloseWithError(err) })
			}
			// Ensure the writer is closed on normal completion.
			defer closeWith(nil)

			// Propagate cancellation to downstream exactly once.
			go func() {
				<-ctx.Done()
				closeWith(ctx.Err())
			}()

			if err := stage(ctx, in, w); err != nil {
				stageErr := fmt.Errorf("stage %d failed: %w", i, err)
				closeWith(stageErr)
				errCh <- stageErr
				cancel() // cancel entire pipeline on error
				return
			}
		}(stage, in, w, i)

		in = r
	}

	// Final stage writes to dst
	if dst == nil {
		dst = io.Discard
	}

	wg.Add(1)
	go func(in io.Reader) {
		defer wg.Done()
		if _, err := io.Copy(dst, in); err != nil {
			errCh <- fmt.Errorf("final copy failed: %w", err)
			cancel()
		}
	}(in)

	wg.Wait()
	close(errCh)

	// Return first error encountered
	for err := range errCh {
		return err
	}
	return nil
}
