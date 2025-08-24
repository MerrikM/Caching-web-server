package ports

import (
	"caching-web-server/internal/model"
	"caching-web-server/internal/security"
	"context"
)

type JWTRepositoryInterface interface {
	FindByUUID(ctx context.Context, uuid string) (*model.RefreshToken, error)
	MarkRefreshTokenUsedByUUID(ctx context.Context, uuid string) error
	SaveRefreshToken(ctx context.Context, token *model.RefreshToken, ipAddress string) error
}

type JWTServiceInterface interface {
	GenerateAccessRefreshTokens(userUUID string) (*model.TokensPair, *model.RefreshToken, error)
	ValidateJWT(tokenString string, secret []byte) (*security.Claims, error)
	ParseAccessToken(tokenStr string) (*security.Claims, error)
}
