package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	_ "database/sql"
	"fmt"
	"github.com/jmoiron/sqlx"
)

type DocumentRepository struct {
	*config.Database
}

func NewDocumentRepository(database *config.Database) *DocumentRepository {
	return &DocumentRepository{database}
}

// Create : сохраняем новый документ
func (r *DocumentRepository) Create(ctx context.Context, exec sqlx.ExtContext, document *model.Document) error {
	query := `
		INSERT INTO documents (uuid, owner_uuid, filename_original, size_bytes, mime_type, sha256, storage_path)
    	VALUES ($1, $2, $3, $4, $5, $6, $7)
	`
	_, err := exec.ExecContext(
		ctx,
		query,
		document.UUID,
		document.OwnerUUID,
		document.FilenameOriginal,
		document.SizeBytes,
		document.MimeType,
		document.Sha256,
		document.StoragePath)

	return err
}

// ShareDocument : добавляет пользователя к документу для совместного доступа
func (r *DocumentRepository) ShareDocument(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string, targetUserUUID string, permission string) error {
	var exists bool
	err := sqlx.GetContext(ctx, exec, &exists, `
        SELECT EXISTS (
            SELECT 1
            FROM documents
            WHERE uuid = $1 AND owner_uuid = $2
        )
    `, documentUUID, ownerUUID)
	if err != nil {
		return err
	}
	if exists == false {
		return fmt.Errorf("доступ запрещен")
	}

	_, err = exec.ExecContext(ctx, `
        INSERT INTO document_shares (document_uuid, target_user_uuid, permission, created_at)
        VALUES ($1, $2, $3, NOW())
        ON CONFLICT (document_uuid, target_user_uuid) DO UPDATE
        SET permission = EXCLUDED.permission, created_at = NOW()
    `, documentUUID, targetUserUUID, permission)

	if err != nil {
		return util.LogError("не удалось сохранить изменения", err)
	}
	return nil
}

// GetByID : возвращаем документ, если юзер владелец или в shares
func (r *DocumentRepository) GetByID(ctx context.Context, exec sqlx.ExtContext, documentUUID string, userID string) (*model.Document, error) {
	query := `
		SELECT d.uuid, d.owner_uuid, d.filename_original, d.size_bytes, d.mime_type,
		       d.sha256, d.storage_path, d.version, d.created_at, d.updated_at, d.deleted_at
		FROM documents AS d
		LEFT JOIN document_shares AS s
		  ON d.uuid = s.document_uuid AND s.target_user_uuid = $2
		WHERE d.uuid = $1 AND (d.owner_uuid = $2 OR s.target_user_uuid IS NOT NULL)
	`

	var document model.Document
	err := sqlx.GetContext(ctx, exec, &document, query, documentUUID, userID)
	if err != nil {
		return nil, err
	}

	return &document, nil
}

// ListByOwner : выдаём список документов владельца (cursor по created_at)
// Вместо стандартного OFFSET/LIMIT метод применяет cursor-based pagination
// cursor здесь — строка, которая хранит значение created_at последнего документа из предыдущей выборки
// При следующем запросе этот cursor передается в БД и ты получаешь документы после (или до) него
func (r *DocumentRepository) ListByOwner(ctx context.Context, exec sqlx.ExtContext, ownerUUID string, cursor string, limit int) ([]model.Document, string, error) {
	queryDesc := `
		SELECT uuid, owner_uuid, filename_original, size_bytes, mime_type,
			   sha256, storage_path, version, created_at, updated_at, deleted_at
		FROM documents
		WHERE owner_uuid = $1 AND deleted_at IS NULL AND created_at < $2
		ORDER BY created_at DESC
		LIMIT $3
	`
	queryAsc := `
		SELECT uuid, owner_uuid, filename_original, storage_path, size_bytes, created_at, version
		FROM documents
		WHERE owner_uuid = $1 AND created_at > $2
		ORDER BY created_at ASC
		LIMIT $3
	`

	docs := []model.Document{}
	var rows *sqlx.Rows
	var err error

	if cursor == "" {
		rows, err = exec.QueryxContext(ctx, queryDesc, ownerUUID, cursor, limit)
	} else {
		rows, err = exec.QueryxContext(ctx, queryAsc, ownerUUID, cursor, limit)
	}
	if err != nil {
		return nil, "", err
	}
	defer rows.Close()

	for rows.Next() {
		var document model.Document
		if err := rows.StructScan(&document); err != nil {
			return nil, "", err
		}
		docs = append(docs, document)
	}

	var nextCursor string
	if len(docs) > 0 {
		nextCursor = docs[len(docs)-1].CreatedAt.Format("2006-01-02T15:04:05.999999Z07:00")
	}

	return docs, nextCursor, nil
}

// Delete : только владелец может удалить документ
func (r *DocumentRepository) Delete(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string) (string, error) {
	query := `
		UPDATE documents
		SET deleted_at = NOW()
		WHERE uuid = $1 AND owner_uuid = $2
		RETURNING storage_path
	`

	var storagePath string
	err := sqlx.GetContext(ctx, exec, &storagePath, query, documentUUID, ownerUUID)
	if err != nil {
		return "", err
	}

	return storagePath, nil
}

func (r *DocumentRepository) BeginTX(ctx context.Context) (sqlx.ExtContext, func() error, error) {
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	return tx, func() error { return tx.Rollback() }, nil
}
