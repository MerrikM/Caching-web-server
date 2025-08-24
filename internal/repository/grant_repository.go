package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/util"
	"context"
	"github.com/jmoiron/sqlx"
)

type GrantDocumentRepository struct {
	database *config.Database
}

func NewGrantDocumentRepository(database *config.Database) *GrantDocumentRepository {
	return &GrantDocumentRepository{database: database}
}

func (r *GrantDocumentRepository) HasAccess(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM documents AS d
			LEFT JOIN document_grants AS s
			  ON d.uuid = s.document_uuid
			 AND s.target_user_uuid = $2
			WHERE d.uuid = $1
			  AND (d.owner_uuid = $2 OR s.target_user_uuid IS NOT NULL)
		)
	`
	var exists bool
	err := sqlx.GetContext(ctx, exec, &exists, query, documentUUID, userUUID)
	if err != nil {
		return false, util.LogError("ошибка проверки доступа", err)
	}
	return exists, nil
}

// AddGrant : добавляет пользователя к документу для совместного доступа
func (r *GrantDocumentRepository) AddGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string, targetUserUUID string) error {
	query := `
		INSERT INTO document_grants (document_uuid, target_user_uuid, created_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (document_uuid, target_user_uuid) DO NOTHING
	`
	_, err := exec.ExecContext(ctx, query, documentUUID, targetUserUUID)

	if err != nil {
		return util.LogError("[DocumentRepo] не удалось предоставить доступ к документу", err)
	}

	return nil
}

func (r *GrantDocumentRepository) CheckOwner(ctx context.Context, exec sqlx.ExtContext, documentUUID, ownerUUID string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (SELECT 1 FROM documents WHERE uuid=$1 AND owner_uuid=$2 AND deleted_at IS NULL)`
	err := sqlx.GetContext(ctx, exec, &exists, query, documentUUID, ownerUUID)
	if err != nil {
		return false, util.LogError("[DocumentRepo] не удалось проверить владельца", err)
	}
	return exists, nil
}

func (r *GrantDocumentRepository) RemoveGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) error {
	_, err := exec.ExecContext(ctx, `
        DELETE FROM document_grants
        WHERE document_uuid = $1 AND target_user_uuid = $2
    `, documentUUID, userUUID)
	if err != nil {
		return util.LogError("[GrantRepo] не удалось удалить доступ к документу", err)
	}
	return nil
}

func (r *GrantDocumentRepository) ListGrants(ctx context.Context, exec sqlx.ExtContext, documentUUID string) ([]string, error) {
	var grants []string
	err := sqlx.SelectContext(ctx, exec, &grants, `
        SELECT u.login
        FROM users AS u
        INNER JOIN document_grants AS g ON u.uuid = g.target_user_uuid
        WHERE g.document_uuid = $1 AND g.deleted_at IS NULL
    `, documentUUID)
	if err != nil {
		return nil, util.LogError("[GrantRepo] не удалось получить список grant", err)
	}
	return grants, nil
}
