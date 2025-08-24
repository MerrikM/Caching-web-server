package security

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/repository"
	"caching-web-server/internal/util"
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

type contextKey string

const (
	UserContextKey contextKey = "user"
)

type Claims struct {
	UserUUID         string `json:"user_uuid"`
	RefreshTokenUUID string `json:"refresh_token_id"`
	IsAdmin          bool   `json:"is_admin,omitempty"`
	jwt.RegisteredClaims
}

type JWTService struct {
	*config.JWTConfig
}

func NewJWTService(cfg *config.JWTConfig) *JWTService {
	return &JWTService{cfg}
}

func (service *JWTService) GenerateAccessRefreshTokens(userUUID string) (*model.TokensPair, *model.RefreshToken, error) {
	refreshToken, refreshTokenStr, err := GenerateRefreshToken()
	if err != nil {
		return nil, nil, util.LogError("ошибка генерации рефреш токена", err)
	}

	refreshToken.UserUUID = userUUID
	timeDuration, err := time.ParseDuration(service.RefreshTokenTTL)
	if err != nil {
		return nil, nil, util.LogError("ошибка парсинга", err)
	}
	refreshToken.ExpireAt = time.Now().Add(timeDuration)

	timeDuration, err = time.ParseDuration(service.AccessTokenTTL)
	if err != nil {
		return nil, nil, util.LogError("ошибка парсинга", err)
	}
	claims := Claims{
		UserUUID:         userUUID,
		RefreshTokenUUID: refreshToken.UUID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(timeDuration)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			Issuer:    "Caching-web-server",
		},
	}

	jwtToken := jwt.NewWithClaims(jwt.SigningMethodHS512, claims)
	accessToken, err := jwtToken.SignedString([]byte(service.SecretKey))
	if err != nil {
		return nil, nil, util.LogError("ошибка подписи токена", err)
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
		return nil, "", util.LogError("ошибка генерации", err)
	}
	refreshUUID := uuid.New().String()
	refreshTokenStr := base64.StdEncoding.EncodeToString(jwtTokenBytes)

	hashedToken, err := bcrypt.GenerateFromPassword([]byte(refreshTokenStr), bcrypt.DefaultCost)
	if err != nil {
		return nil, "", util.LogError("ошибка хэширования", err)
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
		return nil, util.LogError("невалидный токен", err)
	}

	return claims, nil
}

func JWTMiddleware(secretKey []byte, jwtRepository *repository.JWTRepository, jwtService *JWTService, adminToken string) func(handler http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(handleAuthentication(secretKey, jwtRepository, jwtService, adminToken, next))
	}
}

func handleAuthentication(secretKey []byte, jwtRepository *repository.JWTRepository, jwtService *JWTService, adminToken string, next http.Handler) func(writer http.ResponseWriter, request *http.Request) {
	return func(writer http.ResponseWriter, request *http.Request) {
		authorizationHeader := request.Header.Get("Authorization")
		if !strings.HasPrefix(authorizationHeader, "Bearer ") {
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}

		token := strings.TrimPrefix(authorizationHeader, "Bearer ")

		if token == adminToken {
			adminClaims := &Claims{
				UserUUID: "admin",
				IsAdmin:  true,
			}
			req := request.WithContext(context.WithValue(request.Context(), UserContextKey, adminClaims))
			next.ServeHTTP(writer, req)
			return
		}

		claims, err := jwtService.ValidateJWT(token, secretKey)
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

		if refreshToken.Used {
			log.Printf("рефреш токен был использован")
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}

		req := request.WithContext(context.WithValue(request.Context(), UserContextKey, claims))
		next.ServeHTTP(writer, req)
	}
}

func GetClaimsFromContext(ctx context.Context) (*Claims, error) {
	claims, ok := ctx.Value(UserContextKey).(*Claims)
	if !ok || claims == nil {
		return nil, fmt.Errorf("пользователь не авторизован")
	}
	return claims, nil
}
