package handler

import (
	"caching-web-server/internal/model"
	"caching-web-server/internal/model/requestresponse"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type UserHandler struct {
	ports.UserService
}

func NewUserHandler(userService ports.UserService) *UserHandler {
	return &UserHandler{userService}
}

// RegisterUser godoc
// @Summary Регистрация нового пользователя
// @Description Создает нового пользователя с логином и паролем. Требуется токен администратора (токен в config.yaml: "super-secret-admin-token").
// @Tags Users
// @Accept json
// @Produce json
// @Param body body requestresponse.RegisterRequest true "Тело запроса"
// @Success 200 {object} requestresponse.RegisterResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/register [post]
func (h *UserHandler) RegisterUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req requestresponse.RegisterRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	// Метод так же возвращает пару токенов, но по ТЗ только админ может создавать юзеров, так что это просто задел на будущее
	_, err := h.UserService.Register(r.Context(), req.Token, req.Login, req.Password, r.RemoteAddr)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "неверный токен администратора"),
			strings.Contains(err.Error(), "логин должен быть не меньше"),
			strings.Contains(err.Error(), "логин должен содержать"),
			strings.Contains(err.Error(), "database connection не найден"),
			strings.Contains(err.Error(), "пароль"):
			sendErrorResponse(w, 400, "bad request")
		case strings.Contains(err.Error(), "генерации токенов"),
			strings.Contains(err.Error(), "не удалось сохранить refresh"),
			strings.Contains(err.Error(), "не удалось создать хэш"):
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		default:
			sendErrorResponse(w, 500, "неизвестная ошибка")
		}
		return
	}

	resp := requestresponse.RegisterResponse{
		Response: requestresponse.RegisterData{
			Login: req.Login,
		},
	}

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// GetUser godoc
// @Summary Получение информации о пользователе
// @Description Возвращает данные пользователя. Доступен только самому пользователю.
// @Tags Users
// @Produce json
// @Param uuid path string true "UUID пользователя"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.UserResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/users/{uuid} [get]
func (h *UserHandler) GetUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodHead {
		targetUUID := chi.URLParam(r, "uuid")
		if restrictToOwner(w, r, targetUUID) == false {
			return
		}

		_, err := h.UserService.GetUser(r.Context(), targetUUID)
		if err != nil {
			log.Println(err)
			switch {
			case strings.Contains(err.Error(), "не авторизован"),
				strings.Contains(err.Error(), "доступ запрещён"),
				strings.Contains(err.Error(), "database connection не найден"):
				w.WriteHeader(http.StatusForbidden)
			case strings.Contains(err.Error(), "не найден"):
				w.WriteHeader(http.StatusNotFound)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	targetUUID := chi.URLParam(r, "uuid")
	if restrictToOwner(w, r, targetUUID) == false {
		return
	}

	user, err := h.UserService.GetUser(r.Context(), targetUUID)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не авторизован"),
			strings.Contains(err.Error(), "доступ запрещён"),
			strings.Contains(err.Error(), "database connection не найден"):
			sendErrorResponse(w, 403, "доступ запрещён")
		case strings.Contains(err.Error(), "не найден"):
			sendErrorResponse(w, 404, "пользователь не найден")
		default:
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		}
		return
	}

	resp := requestresponse.UserResponse{}
	resp.Data.UUID = user.UUID
	resp.Data.Login = user.Login

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// GetUserHead godoc
// @Summary Получение информации о пользователе
// @Description Возвращает данные пользователя. Доступен только самому пользователю.
// @Tags Users
// @Produce json
// @Param uuid path string true "UUID пользователя"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.UserResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/users/{uuid} [head]
func (h *UserHandler) GetUserHead(w http.ResponseWriter, r *http.Request) {
	h.GetUser(w, r)
}

// UpdateUser godoc
// @Summary Обновление данных пользователя
// @Description Позволяет пользователю обновить свой логин.
// @Tags Users
// @Accept json
// @Produce json
// @Param uuid path string true "UUID пользователя"
// @Param body body requestresponse.UpdateUserRequest true "Тело запроса"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.UpdateUserResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/users/{uuid} [put]
func (h *UserHandler) UpdateUser(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	targetUUID := chi.URLParam(r, "uuid")
	if restrictToOwner(w, r, targetUUID) == false {
		return
	}

	var req requestresponse.UpdateUserRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	updatedUser := &model.User{
		UUID:  targetUUID,
		Login: req.Login,
	}

	if err := h.UserService.UpdateUser(r.Context(), updatedUser); err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не авторизован"),
			strings.Contains(err.Error(), "доступ запрещён"),
			strings.Contains(err.Error(), "database connection не найден"):
			sendErrorResponse(w, 403, "доступ запрещён")
		default:
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		}
		return
	}

	resp := requestresponse.UpdateUserResponse{}
	resp.Response.Login = req.Login

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// UpdatePassword godoc
// @Summary Обновление пароля пользователя
// @Description Позволяет пользователю обновить свой пароль. Доступен только владельцу.
// @Tags Users
// @Accept json
// @Produce json
// @Param uuid path string true "UUID пользователя"
// @Param body body requestresponse.UpdatePasswordRequest true "Тело запроса"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.UpdatePasswordResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/users/{uuid}/password [put]
func (h *UserHandler) UpdatePassword(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	targetUUID := chi.URLParam(r, "uuid")
	if restrictToOwner(w, r, targetUUID) == false {
		return
	}

	var req requestresponse.UpdatePasswordRequest
	if err := decodeJSON(w, r, &req); err != nil {
		return
	}

	if err := h.UserService.UpdatePassword(r.Context(), targetUUID, req.NewPassword); err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не авторизован"),
			strings.Contains(err.Error(), "доступ запрещён"),
			strings.Contains(err.Error(), "database connection не найден"):
			sendErrorResponse(w, 403, "доступ запрещён")
		default:
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		}
		return
	}

	resp := requestresponse.UpdatePasswordResponse{}
	resp.Response.Updated = true

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// DeleteUser godoc
// @Summary Удаление пользователя
// @Description Удаляет пользователя. Доступен только владельцу или администратору.
// @Tags Users
// @Produce json
// @Param uuid path string true "UUID пользователя"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 204 "Пользователь успешно удалён"
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещён"
// @Failure 404 {object} requestresponse.ErrorResponse "Пользователь не найден"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера"
// @Router /api/users/{uuid} [delete]
// @Security BearerAuth
func (h *UserHandler) DeleteUser(w http.ResponseWriter, r *http.Request) {
	targetUUID := chi.URLParam(r, "uuid")
	if restrictToOwner(w, r, targetUUID) == false {
		return
	}

	if err := h.UserService.DeleteUser(r.Context(), targetUUID); err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не авторизован"):
			sendErrorResponse(w, 401, "пользователь не авторизован")
		case strings.Contains(err.Error(), "доступ запрещён"):
			sendErrorResponse(w, 403, "доступ запрещён")
		case strings.Contains(err.Error(), "не найден"):
			sendErrorResponse(w, 404, "пользователь не найден")
		default:
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		}
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ListUsers godoc
// @Summary Получение списка пользователей
// @Description Возвращает список пользователей с постраничной навигацией (cursor-based). Доступно только авторизованным пользователям или администратору.
// @Tags Users
// @Produce json
// @Param cursor query string false "Курсор для пагинации"
// @Param limit query int false "Количество пользователей в списке" default(50) minimum(1) maximum(100)
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.ListUsersResponse
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован"
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещён"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера"
// @Router /api/users [get]
// @Security BearerAuth
func (h *UserHandler) ListUsers(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if r.Method == http.MethodHead {
		authHeader := r.Header.Get("Authorization")
		var adminToken string
		if strings.HasPrefix(authHeader, "Bearer ") {
			adminToken = strings.TrimPrefix(authHeader, "Bearer ")
		}

		_, _, err := h.UserService.ListUsers(r.Context(), adminToken, "", 50)
		if err != nil {
			fmt.Println(err)
			switch {
			case strings.Contains(err.Error(), "не авторизован"):
				w.WriteHeader(http.StatusUnauthorized)
			case strings.Contains(err.Error(), "доступ запрещён"):
				w.WriteHeader(http.StatusForbidden)
			default:
				w.WriteHeader(http.StatusInternalServerError)
			}
			return
		}

		w.WriteHeader(http.StatusOK)
		return
	}

	authHeader := r.Header.Get("Authorization")
	var adminToken string
	if strings.HasPrefix(authHeader, "Bearer ") {
		adminToken = strings.TrimPrefix(authHeader, "Bearer ")
	}

	cursor := r.URL.Query().Get("cursor")
	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 {
			if l > 100 {
				limit = 100
			} else {
				limit = l
			}
		}
	}

	// Вызываем сервис
	users, nextCursor, err := h.UserService.ListUsers(r.Context(), adminToken, cursor, limit)
	if err != nil {
		fmt.Println(err)
		switch {
		case strings.Contains(err.Error(), "не авторизован"):
			sendErrorResponse(w, 401, "пользователь не авторизован")
		case strings.Contains(err.Error(), "доступ запрещён"):
			sendErrorResponse(w, 403, "доступ запрещён")
		default:
			sendErrorResponse(w, 500, "внутренняя ошибка сервера")
		}
		return
	}

	resp := requestresponse.ListUsersResponse{}
	resp.Data.Users = users
	resp.Data.NextCursor = nextCursor

	w.WriteHeader(200)
	json.NewEncoder(w).Encode(resp)
}

// ListUsersHead godoc
// @Summary Получение списка пользователей
// @Description Возвращает список пользователей с постраничной навигацией (cursor-based). Доступно только авторизованным пользователям или администратору.
// @Tags Users
// @Produce json
// @Param cursor query string false "Курсор для пагинации"
// @Param limit query int false "Количество пользователей в списке" default(50) minimum(1) maximum(100)
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.ListUsersResponse
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован"
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещён"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера"
// @Router /api/users [head]
// @Security BearerAuth
func (h *UserHandler) ListUsersHead(w http.ResponseWriter, r *http.Request) {
	h.ListUsers(w, r)
}

// decodeJSON обрабатывает декодирование JSON и возвращает ответ об ошибке, если декодирование не удалось.
func decodeJSON(w http.ResponseWriter, r *http.Request, target interface{}) error {
	if err := json.NewDecoder(r.Body).Decode(target); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		json.NewEncoder(w).Encode(requestresponse.ErrorResponse{
			Error: requestresponse.ErrorDetail{
				Code: 400,
				Text: "invalid request body",
			},
		})
		return err
	}
	return nil
}

// restrictToOwner проверяет, имеет ли пользователь право доступа к ресурсу
func restrictToOwner(w http.ResponseWriter, r *http.Request, targetUUID string) bool {
	claims, ok := r.Context().Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(requestresponse.ErrorResponse{
			Error: requestresponse.ErrorDetail{
				Code: http.StatusUnauthorized,
				Text: "unauthorized",
			},
		})
		return false
	}

	if claims.IsAdmin == false && claims.UserUUID != targetUUID {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusForbidden)
		json.NewEncoder(w).Encode(requestresponse.ErrorResponse{
			Error: requestresponse.ErrorDetail{
				Code: http.StatusForbidden,
				Text: "forbidden",
			},
		})
		return false
	}

	return true
}

// sendErrorResponse отправляет ответ об ошибке JSON с указанным кодом статуса и сообщением
func sendErrorResponse(w http.ResponseWriter, statusCode int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	json.NewEncoder(w).Encode(requestresponse.ErrorResponse{
		Error: requestresponse.ErrorDetail{
			Code: statusCode,
			Text: message,
		},
	})
}
