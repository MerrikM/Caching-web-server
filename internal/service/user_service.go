package service

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"context"
	"fmt"
	"github.com/google/uuid"
	"unicode"
)

type UserService struct {
	userRepository ports.UserRepository
	jwtService     ports.JWTServiceInterface
	jwtRepository  ports.JWTRepositoryInterface
	adminToken     *config.AdminConfig
}

func NewUserService(
	userRepository ports.UserRepository,
	jwtService ports.JWTServiceInterface,
	jwtRepository ports.JWTRepositoryInterface,
	adminToken *config.AdminConfig,
) *UserService {
	return &UserService{
		userRepository: userRepository,
		jwtService:     jwtService,
		jwtRepository:  jwtRepository,
		adminToken:     adminToken,
	}
}

func (s *UserService) Register(ctx context.Context, adminToken string, login string, password string, ipAddress string) (*model.TokensPair, error) {
	if s.adminToken == nil || adminToken != s.adminToken.AdminToken {
		return nil, fmt.Errorf("[UserService] неверный токен администратора")
	}

	if len(login) < 8 {
		return nil, fmt.Errorf("[UserService] логин должен быть не меньше 8 символов")
	}
	for _, c := range login {
		if !unicode.IsLetter(c) && !unicode.IsDigit(c) {
			return nil, fmt.Errorf("[UserService] логин должен содержать только латинские буквы и цифры")
		}
	}

	if err := validatePassword(password); err != nil {
		return nil, fmt.Errorf("[UserService] %w", err)
	}

	hash, err := security.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("[UserService] не удалось создать хэш пароля: %w", err)
	}

	user := &model.User{
		UUID:         uuid.New().String(),
		Login:        login,
		PasswordHash: hash,
	}

	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return nil, fmt.Errorf("[UserService] database connection не найден в context")
	}

	created, err := s.userRepository.CreateUser(ctx, db, user)
	if err != nil {
		return nil, fmt.Errorf("[UserService] ошибка создания пользователя: %w", err)
	}

	tokens, refreshToken, err := s.jwtService.GenerateAccessRefreshTokens(created.UUID)
	if err != nil {
		return nil, fmt.Errorf("[UserService] ошибка генерации токенов: %w", err)
	}

	if err := s.jwtRepository.SaveRefreshToken(ctx, refreshToken, ipAddress); err != nil {
		return nil, fmt.Errorf("[UserService] не удалось сохранить refresh токен: %w", err)
	}

	return tokens, nil
}

func validatePassword(password string) error {
	if len(password) < 8 {
		return fmt.Errorf("пароль должен содержать минимум 8 символов")
	}

	var upperCount, lowerCount, digitCount, specialCount int

	for _, c := range password {
		switch {
		case unicode.IsUpper(c):
			upperCount++
		case unicode.IsLower(c):
			lowerCount++
		case unicode.IsDigit(c):
			digitCount++
		case unicode.IsPunct(c) || unicode.IsSymbol(c):
			specialCount++
		}
	}

	if upperCount == 0 || lowerCount == 0 || (upperCount+lowerCount) < 2 {
		return fmt.Errorf("пароль должен содержать минимум 2 буквы в разных регистрах")
	}
	if digitCount < 1 {
		return fmt.Errorf("пароль должен содержать хотя бы одну цифру")
	}
	if specialCount < 1 {
		return fmt.Errorf("пароль должен содержать хотя бы один специальный символ")
	}

	return nil
}

func (s *UserService) GetUser(ctx context.Context, uuid string) (*model.User, error) {
	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return nil, fmt.Errorf("[UserService] database connection не найден в context")
	}

	claims, err := security.GetClaimsFromContext(ctx)
	if err != nil || claims == nil {
		return nil, fmt.Errorf("[UserService] пользователь не авторизован")
	}

	if claims.IsAdmin == false && claims.UserUUID != uuid {
		return nil, fmt.Errorf("[UserService] доступ запрещён")
	}

	user, err := s.userRepository.FindByUUID(ctx, db, uuid)
	if err != nil || user == nil {
		return nil, fmt.Errorf("[UserService] пользователь не найден")
	}

	return user, nil
}

func (s *UserService) UpdateUser(ctx context.Context, updatedUser *model.User) error {
	claims, err := security.GetClaimsFromContext(ctx)
	if err != nil || claims == nil {
		return fmt.Errorf("[UserService] пользователь не авторизован")
	}

	db, ok := ctx.Value("db").(*config.Database)
	if ok == false {
		return fmt.Errorf("[UserService] database connection не найден в context")
	}

	if claims.UserUUID != updatedUser.UUID {
		return fmt.Errorf("[UserService] доступ запрещён")
	}

	return s.userRepository.UpdateUser(ctx, db, updatedUser)
}

func (s *UserService) UpdatePassword(ctx context.Context, uuid, newPassword string) error {
	claims, err := security.GetClaimsFromContext(ctx)
	if err != nil || claims == nil {
		return fmt.Errorf("[UserService] пользователь не авторизован")
	}

	db, ok := ctx.Value("db").(*config.Database)
	if ok == false {
		return fmt.Errorf("[UserService] database connection не найден в context")
	}

	if claims.UserUUID != uuid {
		return fmt.Errorf("[UserService] доступ запрещён")
	}

	hash, err := security.HashPassword(newPassword)
	if err != nil {
		return err
	}

	return s.userRepository.UpdatePassword(ctx, db, uuid, hash)
}

func (s *UserService) DeleteUser(ctx context.Context, uuid string) error {
	claims, err := security.GetClaimsFromContext(ctx)
	if err != nil || claims == nil {
		return fmt.Errorf("[UserService] пользователь не авторизован")
	}

	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return fmt.Errorf("[UserService] database connection не найден в context")
	}

	if claims.IsAdmin == false && claims.UserUUID != uuid {
		return fmt.Errorf("[UserService] доступ запрещён")
	}

	if err := s.userRepository.DeleteUser(ctx, db, uuid); err != nil {
		return fmt.Errorf("[UserService] пользователь не найден")
	}

	return nil
}

func (s *UserService) ListUsers(ctx context.Context, adminToken string, cursor string, limit int) ([]*model.User, string, error) {
	if s.adminToken != nil && adminToken == s.adminToken.AdminToken {
		return s.listFromRepo(ctx, cursor, limit)
	}

	claims, err := security.GetClaimsFromContext(ctx)
	if err == nil && claims != nil {
		return s.listFromRepo(ctx, cursor, limit)
	}

	return nil, "", fmt.Errorf("[UserService] доступ запрещён: нужен админ или авторизация")
}

func (s *UserService) listFromRepo(ctx context.Context, cursor string, limit int) ([]*model.User, string, error) {
	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return nil, "", fmt.Errorf("[UserService] database connection не найден в context")
	}

	users, nextCursor, err := s.userRepository.ListUsers(ctx, db, cursor, limit)
	if err != nil {
		return nil, "", err
	}

	return users, nextCursor, nil
}
