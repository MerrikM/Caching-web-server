package service

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/notifier"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"caching-web-server/internal/util"
	"context"
	"fmt"
	"log"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type AuthenticationService struct {
	jwtRepoInterface ports.JWTRepositoryInterface
	*config.Config
	jwtServiceInterface ports.JWTServiceInterface
	userRepository      ports.UserRepository
}

func NewAuthenticationService(
	repo ports.JWTRepositoryInterface,
	cfg *config.Config,
	service ports.JWTServiceInterface,
	userInterface ports.UserRepository,
) *AuthenticationService {
	return &AuthenticationService{
		repo,
		cfg,
		service,
		userInterface,
	}
}

func (s *AuthenticationService) Login(ctx context.Context, email, password, userAgent, ipAddress string) (*model.TokensPair, error) {
	user, err := s.userRepository.FindByEmail(ctx, email)
	if err != nil {
		return nil, fmt.Errorf("пользователь не найден: %w", err)
	}

	if !security.CheckPassword(password, user.PasswordHash) {
		return nil, fmt.Errorf("неверный пароль")
	}

	tokens, refreshToken, err := s.jwtServiceInterface.GenerateAccessRefreshTokens(user.UUID)
	if err != nil {
		return nil, fmt.Errorf("ошибка генерации токенов: %w", err)
	}

	refreshToken.UserAgent = userAgent
	refreshToken.IpAddress = ipAddress

	if err := s.jwtRepoInterface.SaveRefreshToken(ctx, refreshToken); err != nil {
		return nil, fmt.Errorf("ошибка сохранения refresh токена: %w", err)
	}

	return tokens, nil
}

// RefreshToken обновляет refresh-токен
// Выполняет следующие требования к операции refresh:
//  1. Операцию refresh можно выполнить только той парой токенов, которая была выдана вместе.
//  2. Запрещает операцию обновления токенов при изменении User-Agent.
//     При этом, после неудачной попытки выполнения операции, деавторизует пользователя,
//     который попытался выполнить обновление токенов.
//  3. При попытке обновления токенов с нового IP отправляет POST-запрос на заданный webhook
//     с информацией о попытке входа со стороннего IP. Запрещать операцию в данном случае не нужно.
//
// Параметры:
//   - ctx: контекст выполнения (для отмены и таймаутов)
//   - userAgent: информацию о бразуере
//   - ipAddress: ip адрес устройства, с которого был выполнен вход
//   - accessToken: текущий access-токен
//   - refreshToken: текущий refresh-токен
//
// Пример:
//
//	tokensPair, err := handler.AuthenticationService.RefreshToken(
//		request.Context(),
//		"PostmanRuntime/7.44.1",
//		"[::1]:52375",
//		"your token",
//		"your refresh token",
//	 )
//
// Возвращает:
//   - model.TokensPair
//   - ошибку, если не удалось обновить токен.
func (s *AuthenticationService) RefreshToken(ctx context.Context, userAgent string, ipAddress string, accessToken string, refreshToken string) (*model.TokensPair, error) {
	claims, err := s.jwtServiceInterface.ValidateJWT(accessToken, []byte(s.Config.JWT.SecretKey))
	if err != nil {
		return nil, util.LogError("не удалось провалидировать токен", err)
	}

	refreshTokenUUID := claims.RefreshTokenUUID
	userUUID := claims.UserUUID

	storedRefreshToken, err := s.jwtRepoInterface.FindByUUID(ctx, refreshTokenUUID)
	if err != nil {
		return nil, util.LogError("не удалось найти рефреш токен", err)
	}
	if storedRefreshToken.Used {
		log.Printf("refresh token %s уже был использован", refreshTokenUUID)
		return nil, fmt.Errorf("невалидный токен")
	}

	if time.Now().UTC().After(storedRefreshToken.ExpireAt) {
		log.Printf("refresh token %s просрочен", refreshTokenUUID)
		return nil, fmt.Errorf("невалидный токен")
	}

	if storedRefreshToken.UserAgent != userAgent {
		if err := s.jwtRepoInterface.MarkRefreshTokenUsedByUUID(ctx, refreshTokenUUID); err != nil {
			log.Printf("не удалось пометить токен использованным: %v", err)
		}
		log.Printf("refresh token %s: попытка обновления с другого User-Agent", refreshTokenUUID)
		return nil, fmt.Errorf("невалидный токен")
	}

	if storedRefreshToken.IpAddress != ipAddress {
		log.Printf("обнаружен вход с нового ip адреса, отправка webhook")
		go func() {
			if err := notifier.NotifyWebhook(s.Config.Webhook.URL, userUUID, ipAddress, storedRefreshToken.IpAddress); err != nil {
				log.Printf("ошибка отправки webhook: %v", err)
			}
		}()
	}

	err = bcrypt.CompareHashAndPassword([]byte(storedRefreshToken.TokenHash), []byte(refreshToken))
	if err != nil {
		return nil, util.LogError("невалидный токен", err)
	}

	if err := s.jwtRepoInterface.MarkRefreshTokenUsedByUUID(ctx, refreshTokenUUID); err != nil {
		return nil, util.LogError("не удалось использовать токен", err)
	}

	tokensPair, newRefreshToken, err := s.jwtServiceInterface.GenerateAccessRefreshTokens(userUUID)
	if err != nil {
		return nil, util.LogError("ошибка генерации токенов", err)
	}

	newRefreshToken.UserAgent = userAgent
	newRefreshToken.IpAddress = ipAddress
	err = s.jwtRepoInterface.SaveRefreshToken(ctx, newRefreshToken)
	if err != nil {
		return nil, util.LogError("не удалось сохранить рефреш токен", err)
	}

	return tokensPair, nil
}

// Logout "деактивирует" пользователя.
// Изменяет статус поля used у refresh-токена и делает его равным true
//
// Параметры:
//   - ctx: контекст выполнения (для отмены и таймаутов)
//   - refreshTokenUUID: UUID рефреш токена из базы даных
//
// Пример:
//
//	err := handler.AuthenticationService.Logout(ctx, "your refresh token uuid")
//
// Возвращает:
//   - ошибку, если не удалось изменить поле used
func (s *AuthenticationService) Logout(ctx context.Context, refreshTokenUUID string) error {
	err := s.jwtRepoInterface.MarkRefreshTokenUsedByUUID(ctx, refreshTokenUUID)
	if err != nil {
		return fmt.Errorf("не удалось использовать токен: %w", err)
	}
	return nil
}
