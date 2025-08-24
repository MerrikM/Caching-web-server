package service_test

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/security"
	"caching-web-server/internal/service"
	"context"
	"errors"
	"github.com/jmoiron/sqlx"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

// ===== MOCKS =====

// MockUserRepository
type MockUserRepository struct {
	mock.Mock
}

func (m *MockUserRepository) FindByEmail(ctx context.Context, exec sqlx.ExtContext, email string) (*model.User, error) {
	args := m.Called(ctx, exec, email)
	if u, ok := args.Get(0).(*model.User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}

// MockJWTService
type MockJWTService struct {
	mock.Mock
}

func (m *MockJWTService) GenerateAccessRefreshTokens(userUUID string) (*model.TokensPair, *model.RefreshToken, error) {
	args := m.Called(userUUID)

	var tokens *model.TokensPair
	if t := args.Get(0); t != nil {
		tokens = t.(*model.TokensPair)
	}

	var refresh *model.RefreshToken
	if r := args.Get(1); r != nil {
		refresh = r.(*model.RefreshToken)
	}

	return tokens, refresh, args.Error(2)
}

// MockJWTRepo
type MockJWTRepo struct {
	mock.Mock
}

func (m *MockJWTRepo) SaveRefreshToken(ctx context.Context, refreshToken *model.RefreshToken, ip string) error {
	args := m.Called(ctx, refreshToken, ip)
	return args.Error(0)
}

// ==== ЗАГЛУШКИ, ЧТОБЫ ИМПЛЕМЕНТИРОВАТЬ ИНТЕРФЕЙСЫ ====
func (m *MockJWTRepo) FindByUUID(ctx context.Context, uuid string) (*model.RefreshToken, error) {
	args := m.Called(ctx, uuid)
	if token, ok := args.Get(0).(*model.RefreshToken); ok {
		return token, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockJWTRepo) MarkRefreshTokenUsedByUUID(ctx context.Context, uuid string) error {
	args := m.Called(ctx, uuid)
	return args.Error(0)
}

func (m *MockUserRepository) CreateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) (*model.User, error) {
	args := m.Called(ctx, exec, user)
	if u, ok := args.Get(0).(*model.User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockUserRepository) FindByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.User, error) {
	args := m.Called(ctx, exec, uuid)
	if u, ok := args.Get(0).(*model.User); ok {
		return u, args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *MockUserRepository) UpdateUser(ctx context.Context, exec sqlx.ExtContext, user *model.User) error {
	args := m.Called(ctx, exec, user)
	return args.Error(0)
}

func (m *MockUserRepository) UpdatePassword(ctx context.Context, exec sqlx.ExtContext, uuid string, newPasswordHash string) error {
	args := m.Called(ctx, exec, uuid, newPasswordHash)
	return args.Error(0)
}

func (m *MockUserRepository) DeleteUser(ctx context.Context, exec sqlx.ExtContext, uuid string) error {
	args := m.Called(ctx, exec, uuid)
	return args.Error(0)
}

func (m *MockUserRepository) ListUsers(ctx context.Context, exec sqlx.ExtContext, cursor string, limit int) ([]*model.User, string, error) {
	args := m.Called(ctx, exec, cursor, limit)
	if users, ok := args.Get(0).([]*model.User); ok {
		return users, args.String(1), args.Error(2)
	}
	return nil, "", args.Error(2)
}

func (m *MockUserRepository) Exists(ctx context.Context, exec sqlx.ExtContext, uuid string) (bool, error) {
	args := m.Called(ctx, exec, uuid)
	return args.Bool(0), args.Error(1)
}

func (m *MockJWTService) ValidateJWT(tokenString string, secret []byte) (*security.Claims, error) {
	args := m.Called(tokenString, secret)
	if claims, ok := args.Get(0).(*security.Claims); ok {
		return claims, args.Error(1)
	}
	return nil, args.Error(1)
}

// ===== HELPERS =====

func newTestAuthService() (*service.AuthenticationService, *MockUserRepository, *MockJWTService, *MockJWTRepo) {
	mockUserRepo := new(MockUserRepository)
	mockJWTService := new(MockJWTService)
	mockJWTRepo := new(MockJWTRepo)

	svc := service.NewAuthenticationService(
		mockJWTRepo,
		&config.AppConfig{}, // если в тестах что-то нужно от конфига — можно заполнить
		mockJWTService,
		mockUserRepo,
	)

	return svc, mockUserRepo, mockJWTService, mockJWTRepo
}

// ===== TESTS =====

// 1. Нет БД в контексте
func TestLogin_NoDBInContext(t *testing.T) {
	svc, _, _, _ := newTestAuthService()

	_, err := svc.Login(context.Background(), "test@example.com", "pass", "agent", "127.0.0.1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database connection not found")
}

// 2. Пользователь не найден
func TestLogin_UserNotFound(t *testing.T) {
	svc, mockUserRepo, _, _ := newTestAuthService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})

	mockUserRepo.On("FindByEmail", ctx, mock.Anything, "test@example.com").
		Return(nil, errors.New("not found"))

	_, err := svc.Login(ctx, "test@example.com", "pass", "agent", "127.0.0.1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "пользователь не найден")
	mockUserRepo.AssertExpectations(t)
}

// 3. Неверный пароль
func TestLogin_WrongPassword(t *testing.T) {
	svc, mockUserRepo, _, _ := newTestAuthService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})

	// создаем юзера с хэшем от "goodpass"
	hash, _ := security.HashPassword("goodpass")
	user := &model.User{UUID: "u1", PasswordHash: hash}

	mockUserRepo.On("FindByEmail", ctx, mock.Anything, "test@example.com").
		Return(user, nil)

	_, err := svc.Login(ctx, "test@example.com", "badpass", "agent", "127.0.0.1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "неверный логин или пароль")
	mockUserRepo.AssertExpectations(t)
}

// 4. Ошибка генерации токенов
func TestLogin_GenerateTokensError(t *testing.T) {
	svc, mockUserRepo, mockJWTService, _ := newTestAuthService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})

	hash, _ := security.HashPassword("goodpass")
	user := &model.User{UUID: "u1", PasswordHash: hash}

	mockUserRepo.On("FindByEmail", ctx, mock.Anything, "test@example.com").
		Return(user, nil)
	mockJWTService.On("GenerateAccessRefreshTokens", "u1").
		Return(nil, nil, errors.New("token error"))

	_, err := svc.Login(ctx, "test@example.com", "goodpass", "agent", "127.0.0.1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ошибка генерации токенов")
	mockUserRepo.AssertExpectations(t)
	mockJWTService.AssertExpectations(t)
}

// 5. Ошибка сохранения refresh токена
func TestLogin_SaveRefreshTokenError(t *testing.T) {
	svc, mockUserRepo, mockJWTService, mockJWTRepo := newTestAuthService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})

	hash, _ := security.HashPassword("goodpass")
	user := &model.User{UUID: "u1", PasswordHash: hash}
	tokens := &model.TokensPair{AccessToken: "acc", RefreshToken: "ref"}
	refresh := &model.RefreshToken{
		UUID:      "r1",
		UserUUID:  "u1",
		TokenHash: "ref",
		ExpireAt:  time.Now().Add(24 * time.Hour),
	}

	mockUserRepo.On("FindByEmail", ctx, mock.Anything, "test@example.com").
		Return(user, nil)
	mockJWTService.On("GenerateAccessRefreshTokens", "u1").
		Return(tokens, refresh, nil)
	mockJWTRepo.On("SaveRefreshToken", ctx, refresh, "127.0.0.1").
		Return(errors.New("db error"))

	_, err := svc.Login(ctx, "test@example.com", "goodpass", "agent", "127.0.0.1")

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ошибка сохранения refresh токена")
	mockUserRepo.AssertExpectations(t)
	mockJWTService.AssertExpectations(t)
	mockJWTRepo.AssertExpectations(t)
}

// 6. Успешный логин
func TestLogin_Success(t *testing.T) {
	svc, mockUserRepo, mockJWTService, mockJWTRepo := newTestAuthService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})

	hash, _ := security.HashPassword("goodpass")
	user := &model.User{UUID: "u1", PasswordHash: hash}
	tokens := &model.TokensPair{AccessToken: "acc", RefreshToken: "ref"}
	refresh := &model.RefreshToken{
		UUID:      "r1",
		UserUUID:  "u1",
		TokenHash: "ref",
		ExpireAt:  time.Now().Add(24 * time.Hour),
	}

	mockUserRepo.On("FindByEmail", ctx, mock.Anything, "test@example.com").
		Return(user, nil)
	mockJWTService.On("GenerateAccessRefreshTokens", "u1").
		Return(tokens, refresh, nil)
	mockJWTRepo.On("SaveRefreshToken", ctx, refresh, "127.0.0.1").
		Return(nil)

	result, err := svc.Login(ctx, "test@example.com", "goodpass", "agent", "127.0.0.1")

	assert.NoError(t, err)
	assert.Equal(t, tokens, result)
	assert.Equal(t, "agent", refresh.UserAgent)
	assert.Equal(t, "127.0.0.1", refresh.IpAddress)

	mockUserRepo.AssertExpectations(t)
	mockJWTService.AssertExpectations(t)
	mockJWTRepo.AssertExpectations(t)
}

func newTestRefreshService() (*service.AuthenticationService, *MockJWTService, *MockJWTRepo) {
	mockJWTService := new(MockJWTService)
	mockJWTRepo := new(MockJWTRepo)

	svc := service.NewAuthenticationService(
		mockJWTRepo, // JWTRepositoryInterface
		&config.AppConfig{ // AppConfig, можно заполнить SecretKey
			JWT: config.JWTConfig{
				SecretKey: "secret",
			},
		},
		mockJWTService, // JWTServiceInterface
		nil,            // UserRepository не нужен для RefreshToken
	)

	return svc, mockJWTService, mockJWTRepo
}

func TestRefreshToken_ValidateJWTError(t *testing.T) {
	svc, mockJWTService, _ := newTestRefreshService()

	ctx := context.Background()

	mockJWTService.On("ValidateJWT", "badtoken", mock.Anything).Return(nil, errors.New("invalid"))

	tokens, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "badtoken", "refresh")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось провалидировать токен")
	mockJWTService.AssertExpectations(t)
}

func TestRefreshToken_RefreshTokenNotFound(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(nil, errors.New("not found"))

	tokens, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "token", "refresh")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "не удалось найти рефреш токен")
	mockJWTService.AssertExpectations(t)
	mockJWTRepo.AssertExpectations(t)
}

func TestRefreshToken_UsedToken(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}
	rt := &model.RefreshToken{Used: true}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(rt, nil)

	tokens, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "token", "refresh")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный токен")
}

func TestRefreshToken_ExpiredToken(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}
	rt := &model.RefreshToken{Used: false, ExpireAt: time.Now().Add(-time.Hour)}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(rt, nil)

	tokens, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "token", "refresh")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный токен")
}

func TestRefreshToken_UserAgentMismatch(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}
	rt := &model.RefreshToken{Used: false, ExpireAt: time.Now().Add(time.Hour), UserAgent: "old-agent"}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(rt, nil)
	mockJWTRepo.On("MarkRefreshTokenUsedByUUID", ctx, "r1").Return(nil)

	tokens, err := svc.RefreshToken(ctx, "new-agent", "127.0.0.1", "token", "refresh")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный токен")
	mockJWTRepo.AssertExpectations(t)
}

func TestRefreshToken_InvalidHash(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}
	rt := &model.RefreshToken{
		Used:      false,
		ExpireAt:  time.Now().Add(time.Hour),
		UserAgent: "agent",
		IpAddress: "127.0.0.1",
		TokenHash: "$2a$10$invalidhashstring...........", // некорректный bcrypt
	}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(rt, nil)

	tokens, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "token", "wrongpass")

	assert.Nil(t, tokens)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "невалидный токен")
}

func TestRefreshToken_Success(t *testing.T) {
	svc, mockJWTService, mockJWTRepo := newTestRefreshService()

	ctx := context.Background()
	claims := &security.Claims{UserUUID: "u1", RefreshTokenUUID: "r1"}

	hash, _ := security.HashPassword("refresh123")
	rt := &model.RefreshToken{
		Used:      false,
		ExpireAt:  time.Now().Add(time.Hour),
		UserAgent: "agent",
		IpAddress: "127.0.0.1",
		TokenHash: hash,
	}

	tokensPair := &model.TokensPair{AccessToken: "acc", RefreshToken: "ref"}
	newRefresh := &model.RefreshToken{}

	mockJWTService.On("ValidateJWT", "token", mock.Anything).Return(claims, nil)
	mockJWTRepo.On("FindByUUID", ctx, "r1").Return(rt, nil)
	mockJWTRepo.On("MarkRefreshTokenUsedByUUID", ctx, "r1").Return(nil)
	mockJWTService.On("GenerateAccessRefreshTokens", "u1").Return(tokensPair, newRefresh, nil)
	mockJWTRepo.On("SaveRefreshToken", ctx, newRefresh, "127.0.0.1").Return(nil)

	result, err := svc.RefreshToken(ctx, "agent", "127.0.0.1", "token", "refresh123")

	assert.NoError(t, err)
	assert.Equal(t, tokensPair, result)
	assert.Equal(t, "agent", newRefresh.UserAgent)
	assert.Equal(t, "127.0.0.1", newRefresh.IpAddress)
}
