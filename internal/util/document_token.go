package util

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"github.com/jmoiron/sqlx"
)

// generateRandomToken : генерирует случайный токен длиной length символов
func generateRandomToken(length int) (string, error) {
	byteLength := (length + 1) / 2 // т.к. hex кодирует 1 байт = 2 символа
	bytes := make([]byte, byteLength)

	_, err := rand.Read(bytes)
	if err != nil {
		return "", LogError("[util] ошибка генерации токена", err)
	}

	return hex.EncodeToString(bytes)[:length], nil
}

func GenerateUniqueToken(ctx context.Context, db *sqlx.DB, length int) (string, error) {
	for {
		token, err := generateRandomToken(length)
		if err != nil {
			return "", err
		}

		var exists bool
		err = db.GetContext(ctx, &exists, `
			SELECT EXISTS (SELECT 1 FROM documents WHERE access_token = $1)
		`, token)

		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", LogError("[util] ошибка проверки токена", err)
		}

		if exists == false {
			return token, nil
		}
	}
}
