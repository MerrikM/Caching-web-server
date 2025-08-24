package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	"time"
)

type UserRepository struct {
	*config.Database
}

func NewUserRepository(database *config.Database) *UserRepository {
	return &UserRepository{database}
}

// CreateUser : сохраняет нового пользователя
func (r *UserRepository) CreateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) (*model.User, error) {
	query := `
	INSERT INTO users (uuid, login, password_hash) 
	VALUES ($1, $2, $3) 
	RETURNING uuid, login, created_at
	`

	createdUser := &model.User{}
	err := exec.QueryRowxContext(ctx, query, user.UUID, user.Login, user.PasswordHash).
		Scan(&createdUser.UUID, &createdUser.Login, &createdUser.CreatedAt)

	if err != nil {
		return nil, util.LogError("[UserRepo] ошибка вставки данных в БД", err)
	}

	return createdUser, nil
}

// FindByUUID : ищет пользователя по UUID
func (r *UserRepository) FindByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.User, error) {
	query := `SELECT uuid, login, password_hash, created_at FROM users WHERE uuid = $1`
	var user model.User
	err := sqlx.GetContext(ctx, exec, &user, query, uuid)
	if err != nil {
		return nil, util.LogError("[UserRepo] не удалось найти пользователя в БД", err)
	}
	return &user, nil
}

// FindByEmail : ищет пользователя по login
func (r *UserRepository) FindByEmail(ctx context.Context, exec sqlx.ExtContext, login string) (*model.User, error) {
	query := `SELECT uuid, login, password_hash, created_at FROM users WHERE login = $1`
	var user model.User
	err := sqlx.GetContext(ctx, exec, &user, query, login)
	if err != nil {
		return nil, util.LogError("[UserRepo] не удалось найти пользователя по login", err)
	}
	return &user, nil
}

// UpdateUser : обновляет поле login
func (r *UserRepository) UpdateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) error {
	query := `
		UPDATE users
		SET login = $2
		WHERE uuid = $1
	`
	_, err := exec.ExecContext(ctx, query, user.UUID, user.Login)
	if err != nil {
		return util.LogError("[UserRepo] не удалось обновить пользователя", err)
	}
	return nil
}

// UpdatePassword : меняет пароль пользователя
func (r *UserRepository) UpdatePassword(ctx context.Context, exec sqlx.ExtContext, uuid, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $2 WHERE uuid = $1`
	_, err := exec.ExecContext(ctx, query, uuid, newPasswordHash)
	if err != nil {
		return util.LogError("[UserRepo] не удалось обновить пароль", err)
	}
	return nil
}

// DeleteUser : удаляет пользователя по его UUID
func (r *UserRepository) DeleteUser(ctx context.Context, exec sqlx.ExtContext, uuid string) error {
	query := `DELETE FROM users WHERE uuid = $1`
	_, err := exec.ExecContext(ctx, query, uuid)
	if err != nil {
		return util.LogError("[UserRepo] не удалось удалить пользователя", err)
	}
	return nil
}

// Exists : проверяет, существует ли пользователь по UUID
func (r *UserRepository) Exists(ctx context.Context, exec sqlx.ExtContext, uuid string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (SELECT 1 FROM users WHERE uuid = $1)`
	err := sqlx.GetContext(ctx, exec, &exists, query, uuid)
	if err != nil {
		return false, util.LogError("[UserRepo] ошибка проверки существования пользователя", err)
	}
	return exists, nil
}

// ListUsers : вывод списка пользователей с cursor-based пагинацией
func (r *UserRepository) ListUsers(ctx context.Context, exec sqlx.ExtContext, cursor string, limit int) ([]*model.User, string, error) {
	query := `
        SELECT uuid, login, password_hash, created_at
        FROM users
        WHERE created_at > $1
        ORDER BY created_at ASC, uuid ASC
        LIMIT $2
    `

	var cursorTime time.Time
	var err error

	if cursor == "" {
		cursorTime = time.Time{}
	} else {
		cursorTime, err = time.Parse(time.RFC3339Nano, cursor)
		if err != nil {
			return nil, "", fmt.Errorf("invalid cursor format: %w", err)
		}
	}

	var users []*model.User
	err = sqlx.SelectContext(ctx, exec, &users, query, cursorTime, limit+1) // +1 для проверки наличия следующей страницы
	if err != nil {
		return nil, "", util.LogError("[UserRepo] не удалось получить список пользователей", err)
	}

	var nextCursor string
	if len(users) > limit {
		users = users[:limit]
		nextCursor = users[len(users)-1].CreatedAt.Format(time.RFC3339Nano)
	}

	return users, nextCursor, nil
}
