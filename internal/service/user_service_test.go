package service_test

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/security"
	srv "caching-web-server/internal/service"
	"context"
	"errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"testing"
)

type MockClaims struct {
	IsAdmin  bool
	UserUUID string
}

func TestUserService_Register(t *testing.T) {
	ctx := context.Background()
	db := &config.Database{} // можно мокать sqlx.DB или оставить заглушку
	ctx = context.WithValue(ctx, "db", db)

	adminConfig := &config.AdminConfig{AdminToken: "secret-admin-token"}

	tests := []struct {
		name        string
		adminToken  string
		login       string
		password    string
		setupMocks  func(u *MockUserRepository, j *MockJWTService, r *MockJWTRepo)
		expectError string
	}{
		{
			name:        "invalid admin token",
			adminToken:  "wrong-token",
			login:       "validlogin",
			password:    "StrongPass123!",
			expectError: "[UserService] неверный токен администратора",
		},
		{
			name:        "short login",
			adminToken:  "secret-admin-token",
			login:       "short",
			password:    "StrongPass123!",
			expectError: "[UserService] логин должен быть не меньше 8 символов",
		},
		{
			name:        "login with invalid chars",
			adminToken:  "secret-admin-token",
			login:       "invalid_!",
			password:    "StrongPass123!",
			expectError: "[UserService] логин должен содержать только латинские буквы и цифры",
		},
		{
			name:       "repository error",
			adminToken: "secret-admin-token",
			login:      "validlogin",
			password:   "StrongPass123!",
			setupMocks: func(u *MockUserRepository, j *MockJWTService, r *MockJWTRepo) {
				u.On("CreateUser", ctx, db, mock.Anything).Return(nil, errors.New("db error"))
			},
			expectError: "[UserService] ошибка создания пользователя: db error",
		},
		{
			name:       "success",
			adminToken: "secret-admin-token",
			login:      "validlogin",
			password:   "StrongPass123!",
			setupMocks: func(u *MockUserRepository, j *MockJWTService, r *MockJWTRepo) {
				createdUser := &model.User{UUID: "user-123", Login: "validlogin"}
				u.On("CreateUser", mock.Anything, mock.Anything, mock.Anything).Return(createdUser, nil)
				j.On("GenerateAccessRefreshTokens", mock.Anything).Return(
					&model.TokensPair{AccessToken: "at", RefreshToken: "rt"},
					&model.RefreshToken{UUID: "rt-123"},
					nil,
				)
				r.On("SaveRefreshToken", mock.Anything, mock.Anything, mock.Anything).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		tt := tt // capture range variable
		t.Run(tt.name, func(t *testing.T) {
			mockUserRepo := new(MockUserRepository)
			mockJWTService := new(MockJWTService)
			mockJWTRepo := new(MockJWTRepo)
			service := srv.NewUserService(mockUserRepo, mockJWTService, mockJWTRepo, adminConfig)

			if tt.setupMocks != nil {
				tt.setupMocks(mockUserRepo, mockJWTService, mockJWTRepo)
			}

			tokens, err := service.Register(ctx, tt.adminToken, tt.login, tt.password, "127.0.0.1")

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, tokens)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, tokens)
			}

			mockUserRepo.AssertExpectations(t)
			mockJWTService.AssertExpectations(t)
			mockJWTRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_GetUser(t *testing.T) {
	db := &config.Database{}

	tests := []struct {
		name        string
		claims      *security.Claims
		uuid        string
		setupMocks  func(mockRepo *MockUserRepository)
		expectError string
	}{
		{
			name:        "user not authorized",
			claims:      nil, // без claims
			uuid:        "user-123",
			expectError: "[UserService] пользователь не авторизован",
		},
		{
			name:        "access denied",
			claims:      &security.Claims{IsAdmin: false, UserUUID: "user-999"},
			uuid:        "user-123",
			expectError: "[UserService] доступ запрещён",
		},
		{
			name:   "user not found",
			claims: &security.Claims{IsAdmin: true},
			uuid:   "user-123",
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("FindByUUID", mock.Anything, mock.Anything, "user-123").
					Return(nil, errors.New("db error"))
			},
			expectError: "[UserService] пользователь не найден",
		},
		{
			name:   "user found",
			claims: &security.Claims{IsAdmin: true},
			uuid:   "user-123",
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("FindByUUID", mock.Anything, mock.Anything, "user-123").
					Return(&model.User{UUID: "user-123", Login: "login"}, nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			service := srv.NewUserService(mockRepo, nil, nil, nil)

			ctx := context.WithValue(context.Background(), "db", db)
			if tt.claims != nil {
				ctx = context.WithValue(ctx, security.UserContextKey, tt.claims)
			}

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo)
			}

			user, err := service.GetUser(ctx, tt.uuid)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, user)
			} else {
				assert.NoError(t, err)
				assert.NotNil(t, user)
				assert.Equal(t, tt.uuid, user.UUID)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_UpdateUser(t *testing.T) {
	db := &config.Database{}

	tests := []struct {
		name        string
		claims      *security.Claims
		user        *model.User
		setupMocks  func(mockRepo *MockUserRepository)
		expectError string
	}{
		{
			name:        "not authorized",
			claims:      nil,
			user:        &model.User{UUID: "user-123"},
			expectError: "[UserService] пользователь не авторизован",
		},
		{
			name:        "access denied",
			claims:      &security.Claims{UserUUID: "user-999"},
			user:        &model.User{UUID: "user-123"},
			expectError: "[UserService] доступ запрещён",
		},
		{
			name:   "db missing",
			claims: &security.Claims{UserUUID: "user-123"},
			user:   &model.User{UUID: "user-123"},
			setupMocks: func(mockRepo *MockUserRepository) {
				// ничего не нужно
			},
			expectError: "[UserService] database connection не найден в context",
		},
		{
			name:   "success",
			claims: &security.Claims{UserUUID: "user-123"},
			user:   &model.User{UUID: "user-123"},
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("UpdateUser", mock.Anything, mock.Anything, mock.Anything).
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			service := srv.NewUserService(mockRepo, nil, nil, nil)

			ctx := context.Background()
			if tt.name != "db missing" {
				ctx = context.WithValue(ctx, "db", db)
			}
			if tt.claims != nil {
				ctx = context.WithValue(ctx, security.UserContextKey, tt.claims)
			}

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo)
			}

			err := service.UpdateUser(ctx, tt.user)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_UpdatePassword(t *testing.T) {
	db := &config.Database{}

	tests := []struct {
		name        string
		claims      *security.Claims
		uuid        string
		newPassword string
		setupMocks  func(mockRepo *MockUserRepository)
		expectError string
	}{
		{
			name:        "not authorized",
			claims:      nil,
			uuid:        "user-123",
			newPassword: "newpass",
			expectError: "[UserService] пользователь не авторизован",
		},
		{
			name:        "access denied",
			claims:      &security.Claims{UserUUID: "user-999"},
			uuid:        "user-123",
			newPassword: "newpass",
			expectError: "[UserService] доступ запрещён",
		},
		{
			name:        "success",
			claims:      &security.Claims{UserUUID: "user-123"},
			uuid:        "user-123",
			newPassword: "newpass",
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("UpdatePassword", mock.Anything, mock.Anything, "user-123", mock.Anything).
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			service := srv.NewUserService(mockRepo, nil, nil, nil)

			ctx := context.WithValue(context.Background(), "db", db)
			if tt.claims != nil {
				ctx = context.WithValue(ctx, security.UserContextKey, tt.claims)
			}

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo)
			}

			err := service.UpdatePassword(ctx, tt.uuid, tt.newPassword)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_DeleteUser(t *testing.T) {
	db := &config.Database{}

	tests := []struct {
		name        string
		claims      *security.Claims
		uuid        string
		setupMocks  func(mockRepo *MockUserRepository)
		expectError string
	}{
		{
			name:        "not authorized",
			claims:      nil,
			uuid:        "user-123",
			expectError: "[UserService] пользователь не авторизован",
		},
		{
			name:        "access denied",
			claims:      &security.Claims{UserUUID: "user-999", IsAdmin: false},
			uuid:        "user-123",
			expectError: "[UserService] доступ запрещён",
		},
		{
			name:   "user not found",
			claims: &security.Claims{UserUUID: "admin", IsAdmin: true},
			uuid:   "user-123",
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("DeleteUser", mock.Anything, mock.Anything, "user-123").
					Return(errors.New("db error"))
			},
			expectError: "[UserService] пользователь не найден",
		},
		{
			name:   "success",
			claims: &security.Claims{UserUUID: "user-123", IsAdmin: false},
			uuid:   "user-123",
			setupMocks: func(mockRepo *MockUserRepository) {
				mockRepo.On("DeleteUser", mock.Anything, mock.Anything, "user-123").
					Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockRepo := new(MockUserRepository)
			service := srv.NewUserService(mockRepo, nil, nil, nil)

			ctx := context.Background()
			ctx = context.WithValue(ctx, "db", db)
			if tt.claims != nil {
				ctx = context.WithValue(ctx, security.UserContextKey, tt.claims)
			}

			if tt.setupMocks != nil {
				tt.setupMocks(mockRepo)
			}

			err := service.DeleteUser(ctx, tt.uuid)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}

func TestUserService_ListUsers(t *testing.T) {
	db := &config.Database{}
	mockRepo := new(MockUserRepository)
	adminConfig := &config.AdminConfig{AdminToken: "secret-admin-token"}

	service := srv.NewUserService(mockRepo, nil, nil, adminConfig)

	tests := []struct {
		name        string
		ctx         context.Context
		adminToken  string
		cursor      string
		limit       int
		setupMocks  func()
		expectCount int
		expectNext  string
		expectError string
	}{
		{
			name:        "access denied",
			ctx:         context.Background(),
			cursor:      "",
			limit:       2,
			expectError: "[UserService] доступ запрещён",
		},
		{
			name: "admin token access",
			ctx: func() context.Context {
				c := context.WithValue(context.Background(), "db", db)
				return c
			}(),
			adminToken: "secret-admin-token",
			cursor:     "",
			limit:      2,
			setupMocks: func() {
				mockRepo.On("ListUsers", mock.Anything, mock.Anything, "", 2).Return(
					[]*model.User{
						{UUID: "u1", Login: "login1"},
						{UUID: "u2", Login: "login2"},
					}, "next-cursor", nil)
			},
			expectCount: 2,
			expectNext:  "next-cursor",
		},
		{
			name: "authorized user access",
			ctx: func() context.Context {
				c := context.WithValue(context.Background(), "db", db)
				c = context.WithValue(c, security.UserContextKey, &security.Claims{UserUUID: "u1"})
				return c
			}(),
			cursor: "",
			limit:  2,
			setupMocks: func() {
				mockRepo.On("ListUsers", mock.Anything, mock.Anything, "", 2).Return(
					[]*model.User{
						{UUID: "u1", Login: "login1"},
						{UUID: "u2", Login: "login2"},
					}, "next-cursor", nil)
			},
			expectCount: 2,
			expectNext:  "next-cursor",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.setupMocks != nil {
				tt.setupMocks()
			}

			users, next, err := service.ListUsers(tt.ctx, tt.adminToken, tt.cursor, tt.limit)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, users)
			} else {
				assert.NoError(t, err)
				assert.Len(t, users, tt.expectCount)
				assert.Equal(t, tt.expectNext, next)
			}

			mockRepo.AssertExpectations(t)
		})
	}
}
