package ports

import (
	"caching-web-server/internal/model"
	"context"
)

// CacheRepository : Redis слой
type CacheRepository interface {
	SetDocument(ctx context.Context, document *model.Document) error
	GetDocument(ctx context.Context, uuid string) (*model.Document, error)
	DeleteDocument(ctx context.Context, uuid string) error
}
