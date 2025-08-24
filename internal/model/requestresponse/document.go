package requestresponse

import (
	"caching-web-server/internal/model"
	"time"
)

// CreateDocumentRequest : представляет мета-данные документа
type CreateDocumentRequest struct {
	Name   string   `json:"name" example:"photo.jpg"`
	File   bool     `json:"file" example:"true"`
	Public bool     `json:"public" example:"false"`
	Token  string   `json:"token" example:"sfuqwejqjoiu93e29"`
	Mime   string   `json:"mime" example:"image/jpg"`
	Grant  []string `json:"grant" example:"['login1','login2']"`
}

// CreateDocumentResponse : описывает ответ при создании документа
type CreateDocumentResponse struct {
	Data CreateDocumentData `json:"data"`
}

type CreateDocumentData struct {
	JSON map[string]interface{} `json:"json,omitempty"`
	File string                 `json:"file"`
}

// GetDocumentResponse : описывает ответ для одного документа
type GetDocumentResponse struct {
	Data      GetDocumentData `json:"data"`
	ExpiresIn string          `json:"expires_in,omitempty"`
}

// DocumentResponse : описывает документ для JSON-ответа
type DocumentResponse struct {
	UUID             string   `json:"id" example:"qwdj1q4o34u34ih759ou1"`
	FilenameOriginal string   `json:"name" example:"photo.jpg"`
	MimeType         string   `json:"mime" example:"image/jpg"`
	IsFile           bool     `json:"file" example:"true"`
	IsPublic         bool     `json:"public" example:"false"`
	CreatedAt        string   `json:"created" example:"2025-08-23T12:34:56Z"`
	GrantLogins      []string `json:"grant" example:"[\"login1\",\"login2\"]"`
	GetURL           string   `json:"get_url,omitempty"`
}

// DocumentResponseFromModel : конвертирует model.Document в DocumentResponse
func DocumentResponseFromModel(doc *model.Document, getURL string) DocumentResponse {
	return DocumentResponse{
		UUID:             doc.UUID,
		FilenameOriginal: doc.FilenameOriginal,
		MimeType:         doc.MimeType,
		IsFile:           doc.IsFile,
		IsPublic:         doc.IsPublic,
		CreatedAt:        doc.CreatedAt.Format(time.RFC3339),
		GrantLogins:      doc.GrantLogins,
		GetURL:           getURL,
	}
}

// ShareDocumentRequest : представляет тело запроса для предоставления доступа
type ShareDocumentRequest struct {
	TargetUserUUID string `json:"target_user_uuid" example:"user-uuid-1234"`
}

// RemoveGrantRequest : представляет тело запроса для удаления гранта доступа к документу
type RemoveGrantRequest struct {
	TargetUserUUID string `json:"target_user_uuid" validate:"required,uuid"`
}

// ResponseMessage : общий ответ для подтверждения действий
type ResponseMessage struct {
	Response map[string]interface{} `json:"response,omitempty"`
	Error    *ErrorResponse         `json:"error,omitempty"`
	Data     interface{}            `json:"data,omitempty"`
}

// ErrorResponseDocument : общий объект ошибки
type ErrorResponseDocument struct {
	Code int    `json:"code" example:"400"`
	Text string `json:"text" example:"описание ошибки"`
}

type GetDocumentData struct {
	Document DocumentResponse `json:"document,omitempty"`
}

// SuccessResponse : стандартный ответ успешного выполнения операции
type SuccessResponse struct {
	Message string `json:"message" example:"Операция выполнена успешно"`
}

// ListDocumentsResponse : ответ API со списком документов
type ListDocumentsResponse struct {
	Data struct {
		Docs []DocumentResponse `json:"docs"`
	} `json:"data"`
	NextCursor string `json:"next_cursor,omitempty" example:"qwdj1q4o34u34ih759ou1"`
	Count      int    `json:"count" example:"10"`
}

type CreateDocumentMeta struct {
	Public bool `json:"public" example:"true"`
}
