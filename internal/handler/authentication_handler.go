package handler

import (
	"caching-web-server/internal/model/requestresponse"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"caching-web-server/internal/service"
	"caching-web-server/internal/util"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"strings"
)

type AuthenticationHandler struct {
	ports.AuthenticationService
	ports.JWTServiceInterface
	ports.JWTRepositoryInterface
}

func NewAuthenticationHandler(
	authenticationService *service.AuthenticationService,
	jwtServiceInterface ports.JWTServiceInterface,
	jwtRepositoryInterface ports.JWTRepositoryInterface,
) *AuthenticationHandler {
	return &AuthenticationHandler{
		authenticationService,
		jwtServiceInterface,
		jwtRepositoryInterface}
}

// Login godoc
// @Summary Аутентификация пользователя
// @Description Получение access токена по логину и паролю
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body requestresponse.LoginRequest true "Тело запроса" example({"login": "user1", "password": "StrongPass123!"})
// @Success 200 {object} requestresponse.LoginResponse "Успешная аутентификация" example({"response": {"token": "access_token_here"}})
// @Failure 400 {object} requestresponse.ErrorResponse "Некорректный JSON или пустые поля" example({"error": "login и password обязательны"})
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован" example({"error": "не удалось авторизовать пользователя"})
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещён" example({"error": "Доступ запрещён"})
// @Failure 404 {object} requestresponse.ErrorResponse "пользователь не найден" example({"error": "пользователь не найден"})
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера" example({"error": "внутренняя ошибка сервера"})
// @Router /api/auth [post]
func (h *AuthenticationHandler) Login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	var req requestresponse.LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, 400, "некорректный JSON")
		return
	}

	if req.Login == "" || req.Password == "" {
		sendErrorResponse(w, 400, "login и pswd обязательны")
		return
	}

	tokens, err := h.AuthenticationService.Login(ctx, req.Login, req.Password, r.UserAgent(), r.RemoteAddr)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "доступ запрещён"):
			util.HandleError(w, "доступ запрещён", http.StatusForbidden)
		case strings.Contains(err.Error(), "не найден"):
			util.HandleError(w, "пользователь не найден", http.StatusNotFound)
		case strings.Contains(err.Error(), "неверный логин или пароль"):
			util.HandleError(w, "неверный логин или пароль", http.StatusUnauthorized)
		default:
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	resp := requestresponse.LoginResponse{}
	resp.Response.Token = tokens.AccessToken

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// GetCurrentUsersUUID godoc
// @Summary Получение UUID текущего пользователя
// @Description Возвращает UUID пользователя, который авторизован в системе
// @Tags Authentication
// @Produce json
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.CurrentUserResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/auth/me [get]
func (h *AuthenticationHandler) GetCurrentUsersUUID(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	ctx := r.Context()
	claims, ok := ctx.Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		err := fmt.Errorf("не авторизован")
		switch err.Error() {
		case "не авторизован":
			sendErrorResponse(w, 401, "не авторизован")
		default:
			sendErrorResponse(w, 500, "неизвестная ошибка")
		}
		return
	}

	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}

	resp := requestresponse.CurrentUserResponse{}
	resp.Response.UserUUID = claims.UserUUID

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// GetCurrentUsersUUIDHead godoc
// @Summary Получение UUID текущего пользователя
// @Description Возвращает UUID пользователя, который авторизован в системе
// @Tags Authentication
// @Produce json
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.CurrentUserResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/auth/me [head]
func (h *AuthenticationHandler) GetCurrentUsersUUIDHead(w http.ResponseWriter, r *http.Request) {
	h.GetCurrentUsersUUID(w, r)
}

// RefreshToken godoc
// @Summary Обновление токенов
// @Description Обновляет пару токенов (access и refresh) по действующему access и refresh токену
// @Tags Authentication
// @Accept json
// @Produce json
// @Param body body requestresponse.RefreshTokenRequest true "Тело запроса"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.RefreshTokenResponse "Новые access и refresh токены" example({"response": {"access_token": "new_access_token_here", "refresh_token": "new_refresh_token_here"}})
// @Failure 400 {object} requestresponse.ErrorResponse "Неверный JSON" example({"error": "неверный JSON"})
// @Failure 401 {object} requestresponse.ErrorResponse "Не авторизован или невалидный токен" example({"error": "не удалось обновить токены"})
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера" example({"error": "внутренняя ошибка сервера"})
// @Router /api/auth/refresh [post]
func (h *AuthenticationHandler) RefreshToken(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
		sendErrorResponse(w, 401, "пустой или неверный заголовок Authorization")
		return
	}

	accessToken := strings.TrimPrefix(authHeader, "Bearer ")

	var req requestresponse.RefreshTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		sendErrorResponse(w, 400, "неверный JSON")
		return
	}

	tokensPair, err := h.AuthenticationService.RefreshToken(ctx, r.UserAgent(), r.RemoteAddr, accessToken, req.RefreshToken)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не удалось провалидировать токен"),
			strings.Contains(err.Error(), "не удалось найти рефреш токен"),
			strings.Contains(err.Error(), "невалидный токен"):
			sendErrorResponse(w, 401, "не удалось обновить токены")
		case strings.Contains(err.Error(), "ошибка генерации токенов"),
			strings.Contains(err.Error(), "не удалось сохранить рефреш токен"),
			strings.Contains(err.Error(), "не удалось использовать токен"):
			log.Println(err)
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		default:
			sendErrorResponse(w, 500, "неизвестная ошибка")
		}
		return
	}

	resp := requestresponse.RefreshTokenResponse{}
	resp.Response.AccessToken = tokensPair.AccessToken
	resp.Response.RefreshToken = tokensPair.RefreshToken

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// Logout godoc
// @Summary Завершение авторизованной сессии
// @Description Инвалидирует refresh-токен и завершает сессию пользователя по access-токену, переданному в URL.
// @Tags Authentication
// @Produce json
// @Param token path string true "Access-токен пользователя (JWT)"
// @Success 200 {object} requestresponse.LogoutResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Security ApiKeyAuth
// @Router /api/auth/{token} [delete]
func (h *AuthenticationHandler) Logout(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	ctx := r.Context()

	accessToken := chi.URLParam(r, "token")
	if accessToken == "" {
		sendErrorResponse(w, http.StatusBadRequest, "токен не указан")
		return
	}

	claims, err := h.JWTServiceInterface.ParseAccessToken(accessToken)
	if err != nil {
		sendErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("невалидный токен: %v", err))
		return
	}

	refreshTokenUUID := claims.RefreshTokenUUID

	if err := h.AuthenticationService.Logout(ctx, refreshTokenUUID); err != nil {
		if strings.Contains(err.Error(), "не удалось использовать токен") {
			sendErrorResponse(w, http.StatusBadRequest, err.Error())
		} else {
			sendErrorResponse(w, http.StatusInternalServerError, "неизвестная ошибка")
		}
		return
	}

	resp := requestresponse.LogoutResponse{
		Response: []requestresponse.LogoutItem{
			{DocumentUUID: refreshTokenUUID, Deleted: true},
		},
	}

	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Println("ошибка кодирования ответа:", err)
	}
}
