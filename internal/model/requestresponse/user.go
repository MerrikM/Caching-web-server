package requestresponse

import "caching-web-server/internal/model"

// RegisterRequest : тело запроса регистрации
type RegisterRequest struct {
	Token    string `json:"token" example:"fixed_admin_token"`
	Login    string `json:"login" example:"newuser123"`
	Password string `json:"password" example:"P@ssw0rd!"`
}

// RegisterResponse : успешный ответ
type RegisterResponse struct {
	Response RegisterData `json:"response"`
}

type RegisterData struct {
	Login string `json:"login"`
}

// ErrorDetail : детальная информация об ошибке
type ErrorDetail struct {
	Code int    `json:"code" example:"400"`
	Text string `json:"text" example:"for example: invalid login or password"`
}

// ErrorResponse : стандартная структура ошибки
type ErrorResponse struct {
	Error ErrorDetail `json:"error"`
}

// UserResponse : успешный ответ с данными пользователя
type UserResponse struct {
	Data struct {
		UUID  string `json:"uuid" example:"123e4567-e89b-12d3-a456-426614174000"`
		Login string `json:"login" example:"user@example.com"`
	} `json:"data"`
}

// UpdateUserRequest : тело запроса на обновление пользователя
type UpdateUserRequest struct {
	Login string `json:"login" example:"newlogin123"`
}

// UpdateUserResponse : успешный ответ
type UpdateUserResponse struct {
	Response struct {
		Login string `json:"login" example:"newlogin123"`
	} `json:"response"`
}

// UpdatePasswordRequest : тело запроса
type UpdatePasswordRequest struct {
	NewPassword string `json:"new_password" example:"P@ssw0rd123"`
}

// UpdatePasswordResponse : успешный ответ
type UpdatePasswordResponse struct {
	Response struct {
		Updated bool `json:"updated" example:"true"`
	} `json:"response"`
}

// ListUsersResponse : успешный ответ
type ListUsersResponse struct {
	Data struct {
		Users      []*model.User `json:"users"`
		NextCursor string        `json:"next_cursor,omitempty"`
	} `json:"data"`
}
