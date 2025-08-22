package service

import (
	"MEDODS_TestProject/config"
	"MEDODS_TestProject/internal/model"
	"MEDODS_TestProject/internal/security"
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/crypto/bcrypt"
)

type MockJWTRepository struct {
	mock.Mock
}

type MockJWTService struct {
	mock.Mock
}

func testConfig() *config.Config {
	return &config.Config{
		JWT: config.JWTConfig{
			SecretKey:       "test-secret",
			AccessTokenTTL:  "15m",
			RefreshTokenTTL: "24h",
		},
		Webhook: config.WebhookConfig{
			URL: "http://example.com/webhook",
		},
	}
}

func (m *MockJWTService) GenerateAccessRefreshTokens(userUUID string) (*model.TokensPair, *model.RefreshToken, error) {
	args := m.Called(userUUID)
	return args.Get(0).(*model.TokensPair), args.Get(1).(*model.RefreshToken), args.Error(2)
}

func (m *MockJWTService) ValidateJWT(tokenString string, secret []byte) (*security.Claims, error) {
	args := m.Called(tokenString, secret)
	claims, _ := args.Get(0).(*security.Claims)
	return claims, args.Error(1)
}

func (m *MockJWTRepository) FindByUUID(ctx context.Context, uuid string) (*model.RefreshToken, error) {
	args := m.Called(ctx, uuid)
	token := args.Get(0)
	if token == nil {
		return nil, args.Error(1)
	}
	return token.(*model.RefreshToken), args.Error(1)
}

func (m *MockJWTRepository) MarkRefreshTokenUsedByUUID(ctx context.Context, uuid string) error {
	return m.Called(ctx, uuid).Error(0)
}

func (m *MockJWTRepository) SaveRefreshToken(ctx context.Context, token *model.RefreshToken) error {
	return m.Called(ctx, token).Error(0)
}

// 1
func TestRefreshToken_InvalidAccessToken(t *testing.T) {
	ctx := context.Background()
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTService: mockJWTService,
		Config:     testConfig(),
	}

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).
		Return(nil, fmt.Errorf("invalid token"))

	_, err := authService.RefreshToken(ctx, "agent", "1.2.3.4", "bad-access-token", "refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось провалидировать токен")
}

// 2
func TestRefreshToken_RefreshTokenNotFound(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).
		Return(&security.Claims{RefreshTokenUUID: refreshUUID, UserUUID: "user-uuid"}, nil)

	mockRepo.On("FindByUUID", ctx, refreshUUID).
		Return(nil, fmt.Errorf("not found"))

	_, err := authService.RefreshToken(ctx, "agent", "1.2.3.4", "access-token", "refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось найти рефреш токен")
}

// 3
func TestRefreshToken_RefreshTokenUsed(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).
		Return(&security.Claims{RefreshTokenUUID: refreshUUID, UserUUID: "user-uuid"}, nil)

	storedToken := &model.RefreshToken{
		UUID: refreshUUID,
		Used: true,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)

	_, err := authService.RefreshToken(ctx, "agent", "1.2.3.4", "access-token", "refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "токен уже был использован")
}

// 4
func TestRefreshToken_RefreshTokenExpired(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).
		Return(&security.Claims{RefreshTokenUUID: refreshUUID, UserUUID: "user-uuid"}, nil)

	storedToken := &model.RefreshToken{
		UUID:     refreshUUID,
		Used:     false,
		ExpireAt: time.Now().Add(-time.Hour), // уже истёк
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)

	_, err := authService.RefreshToken(ctx, "agent", "1.2.3.4", "access-token", "refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "токен просрочен")
}

// 5
func TestRefreshToken_UserAgentMismatch(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"
	userAgent := "agent"

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).
		Return(&security.Claims{RefreshTokenUUID: refreshUUID, UserUUID: "user-uuid"}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		Used:      false,
		ExpireAt:  time.Now().Add(time.Hour),
		UserAgent: "other-agent",
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)
	mockRepo.On("MarkRefreshTokenUsedByUUID", ctx, refreshUUID).Return(nil)

	_, err := authService.RefreshToken(ctx, userAgent, "1.2.3.4", "access-token", "refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "обновление токена запрещено. User-Agent был изменен")
}

// 6
func TestRefreshToken_BcryptCompareFails(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"
	userUUID := "user-uuid"
	userAgent := "agent"
	ip := "1.2.3.4"

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte("correct-refresh-token"), bcrypt.DefaultCost)
	hashedRefresh := string(hashedBytes)

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).Return(&security.Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshUUID,
	}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		UserUUID:  userUUID,
		TokenHash: hashedRefresh,
		Used:      false,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		UserAgent: userAgent,
		IpAddress: ip,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)

	// передаем специально неправильный refresh token, чтобы bcrypt не прошел
	_, err := authService.RefreshToken(ctx, userAgent, ip, "access-token", "wrong-refresh-token")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный токен")
}

// 7
func TestRefreshToken_MarkUsedFails(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"
	userUUID := "user-uuid"
	userAgent := "agent"
	ip := "1.2.3.4"

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte("plain-refresh"), bcrypt.DefaultCost)
	hashedRefresh := string(hashedBytes)

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).Return(&security.Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshUUID,
	}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		UserUUID:  userUUID,
		TokenHash: hashedRefresh,
		Used:      false,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		UserAgent: userAgent,
		IpAddress: ip,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)
	mockRepo.On("MarkRefreshTokenUsedByUUID", ctx, refreshUUID).Return(fmt.Errorf("db error"))

	_, err := authService.RefreshToken(ctx, userAgent, ip, "access-token", "plain-refresh")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось использовать токен")
}

// 8
func TestRefreshToken_GenerateTokensFails(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"
	userUUID := "user-uuid"
	userAgent := "agent"
	ip := "1.2.3.4"

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte("plain-refresh"), bcrypt.DefaultCost)
	hashedRefresh := string(hashedBytes)

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).Return(&security.Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshUUID,
	}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		UserUUID:  userUUID,
		TokenHash: hashedRefresh,
		Used:      false,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		UserAgent: userAgent,
		IpAddress: ip,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)
	mockRepo.On("MarkRefreshTokenUsedByUUID", ctx, refreshUUID).Return(nil)

	mockJWTService.On("GenerateAccessRefreshTokens", userUUID).
		Return((*model.TokensPair)(nil), (*model.RefreshToken)(nil), fmt.Errorf("jwt generation error"))

	_, err := authService.RefreshToken(ctx, userAgent, ip, "access-token", "plain-refresh")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ошибка генерации токенов")
}

// 9
func TestRefreshToken_SaveRefreshTokenFails(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		JWTService:    mockJWTService,
		Config:        testConfig(),
	}

	refreshUUID := "refresh-uuid"
	userUUID := "user-uuid"
	userAgent := "agent"
	ip := "1.2.3.4"

	hashedBytes, _ := bcrypt.GenerateFromPassword([]byte("plain-refresh"), bcrypt.DefaultCost)
	hashedRefresh := string(hashedBytes)

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).Return(&security.Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshUUID,
	}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		UserUUID:  userUUID,
		TokenHash: hashedRefresh,
		Used:      false,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		UserAgent: userAgent,
		IpAddress: ip,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)
	mockRepo.On("MarkRefreshTokenUsedByUUID", ctx, refreshUUID).Return(nil)

	mockJWTService.On("GenerateAccessRefreshTokens", userUUID).
		Return(&model.TokensPair{AccessToken: "new-access", RefreshToken: "new-refresh"}, &model.RefreshToken{UUID: "new-refresh-uuid"}, nil)

	mockRepo.On("SaveRefreshToken", ctx, mock.Anything).
		Return(fmt.Errorf("database error"))

	_, err := authService.RefreshToken(ctx, userAgent, ip, "access-token", "plain-refresh")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось сохранить рефреш токен")
}

// 10
func TestRefreshToken_Success(t *testing.T) {
	ctx := context.Background()
	mockRepo := new(MockJWTRepository)
	mockJWTService := new(MockJWTService)

	authService := &AuthenticationService{
		JWTRepository: mockRepo,
		Config:        testConfig(),
		JWTService:    mockJWTService,
	}

	refreshUUID := "refresh-uuid"
	userUUID := "user-uuid"
	userAgent := "agent"
	ip := "1.2.3.4"

	hashedBytes, err := bcrypt.GenerateFromPassword([]byte("plain-refresh"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("failed to generate bcrypt hash: %v", err)
	}
	hashedRefresh := string(hashedBytes)

	mockJWTService.On("ValidateJWT", mock.Anything, mock.Anything).Return(&security.Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshUUID,
	}, nil)

	storedToken := &model.RefreshToken{
		UUID:      refreshUUID,
		UserUUID:  userUUID,
		TokenHash: hashedRefresh,
		Used:      false,
		ExpireAt:  time.Now().Add(10 * time.Minute),
		UserAgent: userAgent,
		IpAddress: ip,
	}

	mockRepo.On("FindByUUID", ctx, refreshUUID).Return(storedToken, nil)
	mockRepo.On("MarkRefreshTokenUsedByUUID", ctx, refreshUUID).Return(nil)
	mockRepo.On("SaveRefreshToken", ctx, mock.Anything).Return(nil)

	mockJWTService.On("GenerateAccessRefreshTokens", userUUID).Return(
		&model.TokensPair{AccessToken: "new-access", RefreshToken: "new-refresh"},
		&model.RefreshToken{UUID: "new-refresh-uuid"},
		nil,
	)

	tokens, err := authService.RefreshToken(ctx, userAgent, ip, "access-token", "plain-refresh")

	assert.NoError(t, err)
	assert.Equal(t, "new-access", tokens.AccessToken)
	assert.Equal(t, "new-refresh", tokens.RefreshToken)
}
