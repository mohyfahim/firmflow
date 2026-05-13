package storage

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// LocalObjectStore writes under baseDir. Suitable for development; swap for S3 later.
type LocalObjectStore struct {
	baseDir string
}

func NewLocalObjectStore(baseDir string) (*LocalObjectStore, error) {
	if baseDir == "" {
		return nil, fmt.Errorf("storage base path is empty")
	}
	abs, err := filepath.Abs(baseDir)
	if err != nil {
		return nil, fmt.Errorf("storage base path: %w", err)
	}
	if err := os.MkdirAll(abs, 0o750); err != nil {
		return nil, fmt.Errorf("mkdir storage base: %w", err)
	}
	return &LocalObjectStore{baseDir: abs}, nil
}

func (s *LocalObjectStore) resolve(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	full := filepath.Join(s.baseDir, filepath.FromSlash(key))
	rel, err := filepath.Rel(s.baseDir, full)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("invalid storage key")
	}
	return full, nil
}

func (s *LocalObjectStore) Put(ctx context.Context, key string, r io.Reader) (int64, error) {
	full, err := s.resolve(key)
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(filepath.Dir(full), 0o750); err != nil {
		return 0, err
	}

	tmp, err := os.CreateTemp(filepath.Dir(full), ".upload-*")
	if err != nil {
		return 0, err
	}
	tmpPath := tmp.Name()
	cleanup := func() {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
	}

	n, copyErr := io.Copy(tmp, r)
	if closeErr := tmp.Close(); closeErr != nil && copyErr == nil {
		copyErr = closeErr
	}
	if copyErr != nil {
		cleanup()
		return 0, copyErr
	}

	if err := os.Rename(tmpPath, full); err != nil {
		cleanup()
		return 0, err
	}
	_ = ctx // reserved for cancellation hooks
	return n, nil
}

func (s *LocalObjectStore) Open(ctx context.Context, key string) (io.ReadCloser, error) {
	full, err := s.resolve(key)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(full)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("object not found: %w", err)
		}
		return nil, err
	}
	_ = ctx
	return f, nil
}

func (s *LocalObjectStore) Delete(ctx context.Context, key string) error {
	full, err := s.resolve(key)
	if err != nil {
		return err
	}
	err = os.Remove(full)
	if os.IsNotExist(err) {
		return nil
	}
	_ = ctx
	return err
}
