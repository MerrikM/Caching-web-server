package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	"github.com/jmoiron/sqlx"
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
	INSERT INTO users (uuid, email, password_hash) 
	VALUES ($1, $2, $3) 
	RETURNING uuid, email, created_at
	`

	createdUser := &model.User{}
	err := exec.QueryRowxContext(ctx, query, user.UUID, user.Email, user.PasswordHash).
		Scan(&createdUser.UUID, &createdUser.Email, &createdUser.CreatedAt)

	if err != nil {
		return nil, util.LogError("ошибка вставки данных в БД", err)
	}

	return createdUser, nil
}

// FindByUUID : ищет пользователя по UUID
func (r *UserRepository) FindByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.User, error) {
	query := `SELECT uuid, email, password_hash, created_at FROM users WHERE uuid = $1`
	var user model.User
	err := sqlx.GetContext(ctx, exec, &user, query, uuid)
	if err != nil {
		return nil, util.LogError("не удалось найти пользователя в БД", err)
	}
	return &user, nil
}

// FindByEmail : ищет пользователя по email
func (r *UserRepository) FindByEmail(ctx context.Context, exec sqlx.ExtContext, email string) (*model.User, error) {
	query := `SELECT uuid, email, password_hash, created_at FROM users WHERE email = $1`
	var user model.User
	err := sqlx.GetContext(ctx, exec, &user, query, email)
	if err != nil {
		return nil, util.LogError("не удалось найти пользователя по email", err)
	}
	return &user, nil
}

// UpdateUser : обновляет поле email
func (r *UserRepository) UpdateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) error {
	query := `
		UPDATE users
		SET email = $2
		WHERE uuid = $1
	`
	_, err := exec.ExecContext(ctx, query, user.UUID, user.Email)
	if err != nil {
		return util.LogError("не удалось обновить пользователя", err)
	}
	return nil
}

// UpdatePassword : меняет пароль пользователя
func (r *UserRepository) UpdatePassword(ctx context.Context, exec sqlx.ExtContext, uuid, newPasswordHash string) error {
	query := `UPDATE users SET password_hash = $2 WHERE uuid = $1`
	_, err := exec.ExecContext(ctx, query, uuid, newPasswordHash)
	if err != nil {
		return util.LogError("не удалось обновить пароль", err)
	}
	return nil
}

// DeleteUser : удаляет пользователя по его UUID
func (r *UserRepository) DeleteUser(ctx context.Context, exec sqlx.ExtContext, uuid string) error {
	query := `DELETE FROM users WHERE uuid = $1`
	_, err := exec.ExecContext(ctx, query, uuid)
	if err != nil {
		return util.LogError("не удалось удалить пользователя", err)
	}
	return nil
}

// Exists : проверяет, существует ли пользователь по UUID
func (r *UserRepository) Exists(ctx context.Context, exec sqlx.ExtContext, uuid string) (bool, error) {
	var exists bool
	query := `SELECT EXISTS (SELECT 1 FROM users WHERE uuid = $1)`
	err := sqlx.GetContext(ctx, exec, &exists, query, uuid)
	if err != nil {
		return false, util.LogError("ошибка проверки существования пользователя", err)
	}
	return exists, nil
}

// ListUsers : вывод списка пользователей с cursor-based пагинацией
func (r *UserRepository) ListUsers(ctx context.Context, exec sqlx.ExtContext, cursor string, limit int) ([]*model.User, string, error) {
	query := `
		SELECT uuid, email, password_hash, created_at
		FROM users
		WHERE uuid > $1
		ORDER BY uuid
		LIMIT $2
	`
	var users []*model.User
	err := sqlx.SelectContext(ctx, exec, &users, query, cursor, limit)
	if err != nil {
		return nil, "", util.LogError("не удалось получить список пользователей", err)
	}

	var nextCursor string
	if len(users) == limit {
		nextCursor = users[len(users)-1].UUID
	}

	return users, nextCursor, nil
}
