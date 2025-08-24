package ports

import (
	"caching-web-server/internal/model"
	"context"
	"github.com/jmoiron/sqlx"
)

type UserRepository interface {
	CreateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) (*model.User, error)
	FindByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.User, error)
	FindByEmail(ctx context.Context, exec sqlx.ExtContext, email string) (*model.User, error)
	UpdateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) error
	UpdatePassword(ctx context.Context, exec sqlx.ExtContext, uuid, newPasswordHash string) error
	DeleteUser(ctx context.Context, exec sqlx.ExtContext, uuid string) error
	ListUsers(ctx context.Context, exec sqlx.ExtContext, cursor string, limit int) ([]*model.User, string, error)
	Exists(ctx context.Context, exec sqlx.ExtContext, uuid string) (bool, error)
}

type UserService interface {
	Register(ctx context.Context, adminToken string, login string, password string, ipAddress string) (*model.TokensPair, error)
	GetUser(ctx context.Context, uuid string) (*model.User, error)
	UpdateUser(ctx context.Context, updatedUser *model.User) error
	UpdatePassword(ctx context.Context, uuid string, newPassword string) error
	DeleteUser(ctx context.Context, uuid string) error
	ListUsers(ctx context.Context, adminToken string, cursor string, limit int) ([]*model.User, string, error)
}
