package ports

import (
	"caching-web-server/internal/model"
	"context"
)

type AuthenticationService interface {
	Login(ctx context.Context, email, password, userAgent, ipAddress string) (*model.TokensPair, error)
	RefreshToken(ctx context.Context, userAgent, ipAddress, accessToken, refreshToken string) (*model.TokensPair, error)
	Logout(ctx context.Context, refreshTokenUUID string) error
}
