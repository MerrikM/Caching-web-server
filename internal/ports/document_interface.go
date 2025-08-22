package ports

import (
	"caching-web-server/internal/model"
	"context"
	"github.com/jmoiron/sqlx"
)

// DocumentRepository : SQL слой
type DocumentRepository interface {
	Create(ctx context.Context, exec sqlx.ExtContext, document *model.Document) error
	GetByID(ctx context.Context, exec sqlx.ExtContext, docID string, userID string) (*model.Document, error) // учитывает owner/share
	ListByOwner(ctx context.Context, exec sqlx.ExtContext, ownerUUID string, cursor string, limit int) ([]model.Document, string, error)
	Delete(ctx context.Context, exec sqlx.ExtContext, docID string, ownerUUID string) (string, error) // owner only, возвращает storage_path
	ShareDocument(ctx context.Context, exec sqlx.ExtContext, docID string, ownerUUID string, targetUserUUID string, permission string) error
	BeginTX(ctx context.Context) (sqlx.ExtContext, func() error, error)
}

type ShareRepository interface {
	HasReadAccess(ctx context.Context, exec sqlx.ExtContext, docID, userID string) (bool, error)
	HasEditAccess(ctx context.Context, exec sqlx.ExtContext, docID, userID string) (bool, error)
}
