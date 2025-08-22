package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/util"
	"context"
	"github.com/jmoiron/sqlx"
)

type ShareRepository struct {
	database *config.Database
}

func NewShareRepository(database *config.Database) *ShareRepository {
	return &ShareRepository{database: database}
}

// HasReadAccess : true, если юзер владелец или есть в shares с read/edit, иначе false
func (r *ShareRepository) HasReadAccess(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) (bool, error) {
	query := `
		SELECT EXISTS (
			SELECT 1
			FROM documents AS d
			LEFT JOIN document_shares AS s
			  ON d.uuid = s.document_uuid
			 AND s.target_user_uuid = $2
			 AND s.permission IN ('read', 'edit')
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

// HasEditAccess : true, если юзер владелец или есть в shares и permission = 'edit', иначе false
func (r *ShareRepository) HasEditAccess(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) (bool, error) {
	query := `
        SELECT EXISTS (
            SELECT 1
            FROM documents AS d
            LEFT JOIN document_shares AS s
              ON d.uuid = s.document_uuid AND s.target_user_uuid = $2
            WHERE d.uuid = $1 AND (d.owner_uuid = $2 OR s.permission = 'edit')
        )
    `
	var exists bool
	err := sqlx.GetContext(ctx, exec, &exists, query, documentUUID, userUUID)
	if err != nil {
		return false, util.LogError("ошибка проверки доступа", err)
	}
	return exists, err
}
