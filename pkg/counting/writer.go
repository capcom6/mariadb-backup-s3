package counting

import "sync/atomic"

type Writer struct {
	n atomic.Int64
}

func NewWriter() *Writer {
	return new(Writer)
}

func (c *Writer) Write(p []byte) (int, error) {
	n := len(p)
	c.n.Add(int64(n))
	return n, nil
}

func (c *Writer) N() int64 {
	return c.n.Load()
}
