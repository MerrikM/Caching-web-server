package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	_ "database/sql"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"strconv"
	"strings"
)

type DocumentRepository struct {
	*config.Database
}

func NewDocumentRepository(database *config.Database) *DocumentRepository {
	return &DocumentRepository{database}
}

// Create : сохраняем новый документ
func (r *DocumentRepository) Create(ctx context.Context, exec sqlx.ExtContext, document *model.Document) error {
	token, err := util.GenerateUniqueToken(ctx, r.DB, 32)
	if err != nil {
		return err
	}
	document.AccessToken = token

	query := `
		INSERT INTO documents (uuid, owner_uuid, filename_original, size_bytes, mime_type, sha256, storage_path, is_file, is_public, access_token)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = exec.ExecContext(
		ctx,
		query,
		document.UUID,
		document.OwnerUUID,
		document.FilenameOriginal,
		document.SizeBytes,
		document.MimeType,
		document.Sha256,
		document.StoragePath,
		document.IsFile,
		document.IsPublic,
		document.AccessToken,
	)

	if err != nil {
		return util.LogError("[DocumentRepo] не удалось вставить данные в БД", err)
	}

	return nil
}

// GetByUUID : возвращает документ по UUID, если юзер владелец или в shares
func (r *DocumentRepository) GetByUUID(ctx context.Context, exec sqlx.ExtContext, documentUUID string, userID string) (*model.Document, []string, error) {
	query := `
		SELECT d.uuid, d.owner_uuid, d.filename_original, d.size_bytes, d.mime_type,
		       d.sha256, d.storage_path, d.is_file, d.is_public, d.access_token,
		       d.created_at, d.updated_at, d.deleted_at
		FROM documents AS d
		LEFT JOIN document_grants AS g
		  ON d.uuid = g.document_uuid AND g.target_user_uuid = $2
		WHERE d.uuid = $1 AND (d.owner_uuid = $2 OR g.target_user_uuid IS NOT NULL)
	`

	var document model.Document
	err := sqlx.GetContext(ctx, exec, &document, query, documentUUID, userID)
	if err != nil {
		return nil, nil, util.LogError("[DocumentRepo] не удалось получить документ по UUID", err)
	}

	var grants []string
	grantQuery := `
		SELECT u.login
		FROM users u
		INNER JOIN document_grants g ON u.uuid = g.target_user_uuid
		WHERE g.document_uuid = $1
	`
	err = sqlx.SelectContext(ctx, exec, &grants, grantQuery, documentUUID)
	if err != nil {
		return nil, nil, util.LogError("[DocumentRepo] не удалось получить список доступа", err)
	}

	return &document, grants, nil
}

// GetByToken : возвращает публичный документ только по токену
func (r *DocumentRepository) GetByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error) {
	query := `
		SELECT d.uuid, d.owner_uuid, d.filename_original, d.size_bytes, d.mime_type,
		       d.sha256, d.storage_path, d.is_file, d.is_public, d.access_token,
		       d.created_at, d.updated_at, d.deleted_at
		FROM documents AS d
		WHERE d.access_token = $1
	`

	var document model.Document
	err := sqlx.GetContext(ctx, exec, &document, query, token)
	if err != nil {
		return nil, util.LogError("[DocumentRepo] не удалось получить публичный документ по токену", err)
	}

	return &document, nil
}

func (r *DocumentRepository) GetPublicByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.Document, error) {
	query := `
        SELECT d.uuid, d.owner_uuid, d.filename_original, d.size_bytes, d.mime_type,
               d.sha256, d.storage_path, d.is_file, d.is_public, d.access_token,
               d.created_at, d.updated_at, d.deleted_at
        FROM documents AS d
        WHERE d.is_public = true AND d.uuid = $1
    `
	var document model.Document
	err := sqlx.GetContext(ctx, exec, &document, query, uuid)
	if err != nil {
		return nil, util.LogError("[DocumentRepo] не удалось получить публичный документ по UUID", err)
	}
	return &document, nil
}

func (r *DocumentRepository) GetPublicByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error) {
	query := `
        SELECT d.uuid, d.owner_uuid, d.filename_original, d.size_bytes, d.mime_type,
               d.sha256, d.storage_path, d.is_file, d.is_public, d.access_token,
               d.created_at, d.updated_at, d.deleted_at
        FROM documents AS d
        WHERE d.is_public = true AND d.access_token = $1
    `
	var document model.Document
	err := sqlx.GetContext(ctx, exec, &document, query, token)
	if err != nil {
		return nil, util.LogError("[DocumentRepo] не удалось получить публичный документ по токену", err)
	}
	return &document, nil
}

// ListDocuments возвращает список документов пользователя с фильтрацией и сортировкой
func (r *DocumentRepository) ListDocuments(
	ctx context.Context,
	exec sqlx.ExtContext,
	ownerUUID string,
	login string, // для чужих документов
	filterKey string,
	filterValue string,
	limit int,
) ([]model.Document, error) {

	var sb strings.Builder
	args := []interface{}{ownerUUID} // $1 — всегда ownerUUID

	sb.WriteString(`
		SELECT 
			d.uuid,
			d.filename_original,
			d.mime_type,
			true AS is_file,
			d.is_public,
			d.created_at,
			d.owner_uuid,
			d.size_bytes,
			d.sha256,
			d.storage_path,
			d.access_token,
			d.updated_at,
			d.deleted_at
		FROM documents AS d
		LEFT JOIN users AS u ON u.uuid = d.owner_uuid
		WHERE d.deleted_at IS NULL
		  AND d.owner_uuid = $1::uuid
	`)

	// фильтр по чужому логину
	if login != "" {
		sb.WriteString(" AND u.login = $2")
		args = append(args, login)
	}

	// динамические фильтры
	paramIndex := len(args) + 1
	if filterKey != "" && filterValue != "" {
		switch filterKey {
		case "name":
			sb.WriteString(" AND d.filename_original ILIKE $" + strconv.Itoa(paramIndex))
			args = append(args, "%"+filterValue+"%")
		case "mime":
			sb.WriteString(" AND d.mime_type = $" + strconv.Itoa(paramIndex))
			args = append(args, filterValue)
		case "public":
			sb.WriteString(" AND d.is_public = $" + strconv.Itoa(paramIndex))
			args = append(args, strings.ToLower(filterValue) == "true")
		case "created":
			sb.WriteString(" AND DATE(d.created_at) = $" + strconv.Itoa(paramIndex))
			args = append(args, filterValue)
		}
		paramIndex++
	}

	sb.WriteString(" ORDER BY d.filename_original ASC, d.created_at ASC")

	// лимит
	if limit > 0 {
		sb.WriteString(" LIMIT $" + strconv.Itoa(len(args)+1))
		args = append(args, limit)
	}

	docs := []model.Document{}
	if err := sqlx.SelectContext(ctx, exec, &docs, sb.String(), args...); err != nil {
		return nil, util.LogError("[DocumentRepo] ошибка запроса списка документов", err)
	}

	return docs, nil
}

// Delete : только владелец может удалить документ
func (r *DocumentRepository) Delete(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string) (string, error) {
	query := `
		DELETE FROM documents
		WHERE uuid = $1 AND owner_uuid = $2
		RETURNING uuid
	`

	var deletedUUID string
	err := sqlx.GetContext(ctx, exec, &deletedUUID, query, documentUUID, ownerUUID)
	if err != nil {
		return "", util.LogError("[DocumentRepo] не удалось удалить документ", err)
	}

	return deletedUUID, nil
}

func (r *DocumentRepository) BeginTX(ctx context.Context) (sqlx.ExtContext, func() error, error) {
	tx, err := r.DB.BeginTxx(ctx, nil)
	if err != nil {
		return nil, nil, err
	}
	return tx, func() error { return tx.Rollback() }, nil
}
