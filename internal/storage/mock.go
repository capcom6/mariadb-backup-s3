package storage

import (
	"context"
	"io"

	"github.com/stretchr/testify/mock"
)

type MockBackend struct {
	mock.Mock
}

func (m *MockBackend) Upload(ctx context.Context, filename string, data io.Reader) error {
	args := m.Called(ctx, filename, data)
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) Download(ctx context.Context, filename string, data io.Writer) error {
	args := m.Called(ctx, filename, data)
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) Delete(ctx context.Context, filename string) error {
	args := m.Called(ctx, filename)
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) List(ctx context.Context) ([]string, error) {
	args := m.Called(ctx)
	s, _ := args.Get(0).([]string)
	return s, args.Error(1) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) UploadBytes(ctx context.Context, filename string, data []byte) error {
	args := m.Called(ctx, filename, data)
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) DownloadBytes(ctx context.Context, filename string) ([]byte, error) {
	args := m.Called(ctx, filename)
	b, _ := args.Get(0).([]byte)
	return b, args.Error(1) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) Lock(ctx context.Context, filename string) (Locker, error) {
	args := m.Called(ctx, filename)
	l, _ := args.Get(0).(Locker)
	return l, args.Error(1) //nolint:wrapcheck // errors must pass through
}

func (m *MockBackend) Close() error {
	args := m.Called()
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}

type MockLocker struct {
	mock.Mock
}

func (m *MockLocker) Unlock(ctx context.Context) error {
	args := m.Called(ctx)
	return args.Error(0) //nolint:wrapcheck // errors must pass through
}
