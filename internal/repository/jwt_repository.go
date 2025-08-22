package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	"database/sql"
	"errors"
)

type JWTRepository struct {
	*config.Database
}

func NewJWTRepository(database *config.Database) *JWTRepository {
	return &JWTRepository{database}
}

// SaveRefreshToken сохраняет refresh-токен в базе данных
// Возвращает ошибку, если операция не удалась
func (r *JWTRepository) SaveRefreshToken(ctx context.Context, refreshToken *model.RefreshToken) error {
	query := `INSERT INTO refresh_tokens (uuid, user_uuid, token_hash, expire_at, used, user_agent, ip_address) 
				VALUES ($1, $2, $3, $4, $5, $6, $7)
	`

	_, err := r.DB.ExecContext(ctx, query,
		refreshToken.UUID,
		refreshToken.UserUUID,
		refreshToken.TokenHash,
		refreshToken.ExpireAt,
		refreshToken.Used,
		refreshToken.UserAgent,
		refreshToken.IpAddress,
	)

	if err != nil {
		return util.LogError("ошибка вставки данных в БД", err)
	}

	return nil
}

// MarkRefreshTokenUsedByUUID изменяет поле used, делая его равным true
// Возвращает ошибку, если не получилось изменить поле
func (r *JWTRepository) MarkRefreshTokenUsedByUUID(ctx context.Context, refreshTokenUUID string) error {
	query := `UPDATE refresh_tokens SET used = TRUE WHERE uuid = $1 AND used = FALSE`

	result, err := r.DB.ExecContext(ctx, query, refreshTokenUUID)
	if err != nil {
		return util.LogError("не удалось обновить рефреш токен", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return util.LogError("не удалось проверить, обновлен ли токен", err)
	}
	if rowsAffected == 0 {
		return util.LogError("не удалось найти токен для его обновления", err)
	}

	return nil
}

// FindByUUID ищет refresh-токен в базе данных
// Возвращает модель model.RefreshToken или ошибку, если не удалось найти токен
func (r *JWTRepository) FindByUUID(ctx context.Context, refreshTokenUUID string) (*model.RefreshToken, error) {
	query := `SELECT uuid, user_uuid, token_hash, expire_at, used, user_agent, ip_address FROM refresh_tokens WHERE uuid = $1`

	refreshToken := &model.RefreshToken{}

	err := r.DB.QueryRowContext(ctx, query, refreshTokenUUID).Scan(
		&refreshToken.UUID,
		&refreshToken.UserUUID,
		&refreshToken.TokenHash,
		&refreshToken.ExpireAt,
		&refreshToken.Used,
		&refreshToken.UserAgent,
		&refreshToken.IpAddress,
	)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, util.LogError("токен не был найден", err)
		}
		return nil, util.LogError("ошибка при выполнении запроса", err)
	}

	return refreshToken, nil
}
