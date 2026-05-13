package storage

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// ObjectStore persists opaque blobs under logical keys. Keys are internal
// (e.g. projects/{projectID}/firmware/{firmwareID}/blob), never end-user paths.
type ObjectStore interface {
	Put(ctx context.Context, key string, r io.Reader) (written int64, err error)
	Open(ctx context.Context, key string) (io.ReadCloser, error)
	Delete(ctx context.Context, key string) error
}

func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("storage key is empty")
	}
	if strings.Contains(key, "..") {
		return fmt.Errorf("invalid storage key")
	}
	if filepath.IsAbs(key) || strings.HasPrefix(key, "/") || strings.HasPrefix(key, "\\") {
		return fmt.Errorf("invalid storage key")
	}
	return nil
}
