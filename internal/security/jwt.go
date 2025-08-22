package security

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/repository"
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

type Claims struct {
	UserUUID         string `json:"user_uuid"`
	RefreshTokenUUID string `json:"refresh_token_id"`
	jwt.RegisteredClaims
}

type JWTService struct {
	*config.Config
}

func NewJWTService(cfg *config.Config) *JWTService {
	return &JWTService{cfg}
}

func (service *JWTService) GenerateAccessRefreshTokens(userUUID string) (*model.TokensPair, *model.RefreshToken, error) {
	refreshToken, refreshTokenStr, err := GenerateRefreshToken()
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка генерации рефреш токена: %w", err)
	}

	refreshToken.UserUUID = userUUID
	timeDuration, err := time.ParseDuration(service.Config.JWT.RefreshTokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка парсинга: %w", err)
	}
	refreshToken.ExpireAt = time.Now().Add(timeDuration)

	timeDuration, err = time.ParseDuration(service.Config.JWT.AccessTokenTTL)
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка парсинга: %w", err)
	}
	claims := Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshToken.UUID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(timeDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "MEDODS_TestProject",
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	accessToken, err := jwtToken.SignedString([]byte(service.Config.JWT.SecretKey))
	if err != nil {
		return nil, nil, fmt.Errorf("ошибка подписи токена: %w", err)
	}

	return &model.TokensPair{
		AccessToken:  accessToken,
		RefreshToken: refreshTokenStr,
	}, refreshToken, nil
}

func GenerateRefreshToken() (*model.RefreshToken, string, error) {
	jwtTokenBytes := make([]byte, 32)
	_, err := rand.Read(jwtTokenBytes)
	if err != nil {
		return nil, "", fmt.Errorf("ошибка генерации: %w", err)
	}
	refreshUUID := uuid.New().String()
	refreshTokenStr := base64.StdEncoding.EncodeToString(jwtTokenBytes)

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(refreshTokenStr), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", fmt.Errorf("ошибка хэширования: %w", err)
	}

	// refreshTokenStr отдается клиенту
	// hashedToken сохраняется в БД
	return &model.RefreshToken{
		UUID:      refreshUUID,
		TokenHash: string(hashedToken),
		Used:      false,
	}, refreshTokenStr, nil
}

func (service *JWTService) ValidateJWT(jwtTokenStr string, secretKey []byte) (*Claims, error) {
	var claims = &Claims{}

	jwtToken, err := jwt.ParseWithClaims(jwtTokenStr, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Header["alg"] != jwt.SigningMethodHS512.Alg() {
			return nil, fmt.Errorf("неверный способ подписи токена: %v", token.Header["alg"])
		}
		return secretKey, nil
	})

	if err != nil || jwtToken.Valid == false {
		return nil, fmt.Errorf("невалидный токен: %w", err)
	}

	return claims, nil
}

func JWTMiddleware(secretKey []byte, jwtRepository *repository.JWTRepository, jwtService *JWTService) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(handleAuthentication(secretKey, jwtRepository, jwtService, next))
	}
}

func handleAuthentication(secretKey []byte, jwtRepository *repository.JWTRepository, jwtService *JWTService, next http.Handler) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		authorizationHeader := request.Header.Get("Authorization")
		if strings.HasPrefix(authorizationHeader, "Bearer ") == false {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}

		jwtTokenStr := strings.TrimPrefix(authorizationHeader, "Bearer ")

		claims, err := jwtService.ValidateJWT(jwtTokenStr, secretKey)
		if err != nil {
			log.Printf("невалидный токен: %v", err)
			http.Error(writer, "невалидный токен", http.StatusUnauthorized)
			return
		}

		refreshToken, err := jwtRepository.FindByUUID(request.Context(), claims.RefreshTokenUUID)
		if err != nil {
			log.Printf("рефреш токен не найден: %v", err)
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		if refreshToken.Used == true {
			log.Printf("рефреш токен был использован: %v", err)
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}

		reuqest := request.WithContext(context.WithValue(request.Context(), "user", claims))
		next.ServeHTTP(writer, reuqest)
	}
}
