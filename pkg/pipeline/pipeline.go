package pipeline

import (
	"fmt"
	"io"
	"sync"
)

// StageFunc defines a processing step from Reader to Writer
type StageFunc func(io.Reader, io.Writer) error

// Run connects stages using io.Pipe and runs them in parallel
func Run(src io.Reader, dst io.Writer, stages ...StageFunc) error {
	var wg sync.WaitGroup
	var err error

	// Current input stream
	in := src

	for i, stage := range stages {
		r, w := io.Pipe()

		wg.Add(1)
		go func(stage StageFunc, in io.Reader, w *io.PipeWriter, i int) {
			defer wg.Done()
			defer w.Close()

			if err := stage(in, w); err != nil {
				_ = w.CloseWithError(fmt.Errorf("stage %d failed: %w", i, err))
			}
		}(stage, in, w, i)

		in = r // output becomes next input
	}

	// Final stage writes to dst
	wg.Add(1)
	go func(in io.Reader) {
		defer wg.Done()
		_, _ = io.Copy(dst, in)
	}(in)

	wg.Wait()
	return err
}
