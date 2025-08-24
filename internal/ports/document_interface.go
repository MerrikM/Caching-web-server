package ports

import (
	"caching-web-server/internal/model"
	"context"
	"github.com/jmoiron/sqlx"
)

// DocumentRepository : SQL слой
type DocumentRepository interface {
	Create(ctx context.Context, exec sqlx.ExtContext, document *model.Document) error
	GetByUUID(ctx context.Context, exec sqlx.ExtContext, documentUUID string, userID string) (*model.Document, []string, error)
	GetByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error)
	GetPublicByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.Document, error)
	GetPublicByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error)
	ListDocuments(ctx context.Context, exec sqlx.ExtContext, ownerUUID, login, filterKey, filterValue string, limit int) ([]model.Document, error)
	Delete(ctx context.Context, exec sqlx.ExtContext, docID string, ownerUUID string) (string, error)
	BeginTX(ctx context.Context) (sqlx.ExtContext, func() error, error)
}

type GrantDocumentRepository interface {
	AddGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string, targetUserUUID string) error
	RemoveGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) error
	ListGrants(ctx context.Context, exec sqlx.ExtContext, documentUUID string) ([]string, error)
	CheckOwner(ctx context.Context, exec sqlx.ExtContext, documentUUID, ownerUUID string) (bool, error)
	HasAccess(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) (bool, error)
}

type DocumentService interface {
	CreateDocument(ctx context.Context, document *model.Document) (string, error)
	GetDocumentByUUID(ctx context.Context, documentUUID string) (*model.GetDocumentResult, error)
	GetPublicDocument(ctx context.Context, documentUUID, token string) (*model.GetDocumentResult, error)
	GetDocumentByToken(ctx context.Context, token string) (*model.GetDocumentResult, error)
	ShareDocument(ctx context.Context, documentUUID, ownerUUID string, targetUserUUID string) error
	DeleteDocument(ctx context.Context, documentUUID, userUUID string) (map[string]bool, error)
	ListDocuments(ctx context.Context, userUUID, login, filterKey, filterValue string, limit int) ([]model.DocumentResponse, string, error)
	AddGrant(ctx context.Context, documentUUID, ownerUUID, targetUserUUID string) error
	RemoveGrant(ctx context.Context, documentUUID, ownerUUID, targetUserUUID string) error
}
