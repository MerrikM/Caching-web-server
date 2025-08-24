package requestresponse

// LoginRequest : тело запроса на аутентификацию
type LoginRequest struct {
	Login    string `json:"login" example:"user1"`
	Password string `json:"password" example:"P@ssw0rd123"`
}

// LoginResponse : ответ успешную на аутентификацию
type LoginResponse struct {
	Response struct {
		Token string `json:"token" example:"sfuqwejqjoiu93e29"`
	} `json:"response"`
}

// CurrentUserResponse : информация о текущем пользователе
type CurrentUserResponse struct {
	Response struct {
		UserUUID string `json:"user_uuid" example:"b6a1e1c4-4b1d-4f1e-8b29-1234567890ab"`
	} `json:"response"`
}

// RefreshTokenRequest : запрос на обновление пары токенов
type RefreshTokenRequest struct {
	RefreshToken string `json:"refresh_token" example:"sfuqwejqjoiu93e29"`
}

// RefreshTokenResponse : ответ на успешный запрос
type RefreshTokenResponse struct {
	Response struct {
		AccessToken  string `json:"access_token" example:"eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9..."`
		RefreshToken string `json:"refresh_token" example:"sfuqwejqjoiu93e29"`
	} `json:"response"`
}

// LogoutItem : элемент ответа на logout
type LogoutItem struct {
	DocumentUUID string `json:"document_uuid" example:"qwdj1q4o34u34ih759ou1"`
	Deleted      bool   `json:"deleted" example:"true"`
}

// LogoutResponse : ответ на завершение сессии
type LogoutResponse struct {
	Response []LogoutItem `json:"response"`
}
