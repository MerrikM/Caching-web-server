package ports

import (
	"context"
	"time"
)

// S3Storage : для S3
type S3Storage interface {
	GeneratePresignedGetURL(ctx context.Context, key string, expire time.Duration) (string, error)
	GeneratePresignedPutURL(ctx context.Context, key string, expire time.Duration) (string, error)
	DeleteObject(ctx context.Context, key string) error
}
