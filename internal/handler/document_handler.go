package handler

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	requestresponse "caching-web-server/internal/model/requestresponse"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"caching-web-server/internal/util"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type DocumentHandler struct {
	ports.DocumentService
	cfg *config.TTL
}

func NewDocumentHandler(documentService ports.DocumentService, cfg *config.TTL) *DocumentHandler {
	return &DocumentHandler{documentService, cfg}
}

// CreateDocument godoc
// @Summary Загрузка нового документа
// @Description Загружает файл и его мета-данные, поддерживает multipart/form-data.
// Метаданные могут содержать имя файла, публичность, MIME-тип и другие параметры.
// @Tags Documents
// @Accept multipart/form-data
// @Produce json
// @Param public formData string true "Введите true, чтобы документ был публичным, либо false, чтобы вы сами давали доступ"
// @Param file formData file true "Файл документа"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 202 {object} requestresponse.CreateDocumentResponse "Успешный ответ, содержит данные документа и pre-signed URL"
// @Failure 400 {object} requestresponse.ErrorResponse "Неверный формат запроса или мета-данных"
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера"
// @Router /api/docs [post]
func (h *DocumentHandler) CreateDocument(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	if err := r.ParseMultipartForm(1 << 20); err != nil {
		util.HandleError(w, "неверный формат запроса", http.StatusBadRequest)
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		util.HandleError(w, "файл не найден в запросе", http.StatusBadRequest)
		return
	}
	defer file.Close()

	claims, ok := ctx.Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		util.HandleError(w, "пользователь не авторизован", http.StatusUnauthorized)
		return
	}

	fileBytes, err := io.ReadAll(file)
	if err != nil {
		util.HandleError(w, "ошибка чтения файла", http.StatusInternalServerError)
		return
	}

	hash := sha256.Sum256(fileBytes)
	sha256Hash := hex.EncodeToString(hash[:])
	size := int64(len(fileBytes))
	mimeType := header.Header.Get("Content-Type")
	if mimeType == "" {
		mimeType = "application/octet-stream"
	}

	fileExt := filepath.Ext(header.Filename)
	fileName := strings.TrimSuffix(header.Filename, fileExt)
	storagePath := fmt.Sprintf("users/%s/documents/%s-%s%s",
		claims.UserUUID,
		url.PathEscape(fileName),
		uuid.New().String()[:8],
		fileExt,
	)

	isPublic := false
	if publicStr := r.FormValue("public"); publicStr != "" {
		if parsed, err := strconv.ParseBool(publicStr); err != nil {
			util.HandleError(w, "неверный формат public (должно быть true/false)", http.StatusBadRequest)
			return
		} else {
			isPublic = parsed
		}
	}

	document := &model.Document{
		UUID:             uuid.New().String(),
		OwnerUUID:        claims.UserUUID,
		FilenameOriginal: header.Filename,
		SizeBytes:        size,
		MimeType:         mimeType,
		Sha256:           sha256Hash,
		StoragePath:      storagePath,
		IsFile:           true,
		IsPublic:         isPublic,
		CreatedAt:        time.Now(),
		UpdatedAt:        time.Now(),
	}

	putURL, err := h.DocumentService.CreateDocument(ctx, document)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не удалось URL"),
			strings.Contains(err.Error(), "не удалось сохранить документ"),
			strings.Contains(err.Error(), "database connection не найден"):
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	tmpFile, err := saveTempFile(fileBytes, header.Filename)
	if err != nil {
		util.HandleError(w, "ошибка сохранения файла", http.StatusInternalServerError)
		return
	}

	uploader := util.NewS3Uploader()
	uploader.UploadFileAsync(putURL, tmpFile)

	metaMap := map[string]interface{}{
		"uuid":   document.UUID,
		"name":   document.FilenameOriginal,
		"mime":   document.MimeType,
		"size":   document.SizeBytes,
		"sha256": document.Sha256,
		"path":   document.StoragePath,
		"putURL": putURL,
		"file":   document.IsFile,
		"public": isPublic,
	}

	response := requestresponse.CreateDocumentResponse{
		Data: requestresponse.CreateDocumentData{
			JSON: metaMap,
			File: header.Filename,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(response)

	// Асинхронный мониторинг
	go h.monitorUpload(document.UUID, uploader)
}

// saveTempFile : сохраняет файл во временную директорию
func saveTempFile(data []byte, filename string) (string, error) {
	// создаем временную директорию если нужно
	tmpDir := os.TempDir()
	uploadDir := filepath.Join(tmpDir, "uploads")

	if err := os.MkdirAll(uploadDir, 0755); err != nil {
		return "", fmt.Errorf("ошибка создания директории: %w", err)
	}

	uniqueName := fmt.Sprintf("%d_%s", time.Now().UnixNano(), filename)
	tmpFile := filepath.Join(uploadDir, uniqueName)

	err := os.WriteFile(tmpFile, data, 0644)
	if err != nil {
		return "", fmt.Errorf("ошибка записи файла: %w", err)
	}

	return tmpFile, nil
}

func (h *DocumentHandler) monitorUpload(documentUUID string, uploader *util.S3Uploader) {
	for {
		select {
		case err, ok := <-uploader.Errors():
			if ok == false {
				return
			}
			log.Printf("[DocumentHandler/MonitorUpload] ошибка загрузки документа %s: %v", documentUUID, err)

		case progress, ok := <-uploader.Progress():
			if ok == false {
				return
			}
			if progress == -1 {
				log.Printf("[DocumentHandler/MonitorUpload] документ %s успешно загружен", documentUUID)
			}

		case <-time.After(30 * time.Minute):
			log.Printf("[DocumentHandler/MonitorUpload] Таймаут загрузки документа %s", documentUUID)
			return
		}
	}
}

// GetDocument godoc
// @Summary Получение документа по ID
// @Description Возвращает документ в JSON или файл по ссылке.
// @Tags Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs/{doc_id} [get]
func (h *DocumentHandler) GetDocument(w http.ResponseWriter, r *http.Request) {
	docUUID := chi.URLParam(r, "doc_id")
	if docUUID == "" {
		err := fmt.Errorf("ID документа обязателен")
		switch err.Error() {
		case "ID документа обязателен":
			util.HandleError(w, err.Error(), http.StatusBadRequest)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	result, err := h.DocumentService.GetDocumentByUUID(r.Context(), docUUID)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "документ не найден"):
			util.HandleError(w, "документ не найден", http.StatusNotFound)
		case strings.Contains(err.Error(), "доступ запрещён"):
			util.HandleError(w, "доступ запрещён", http.StatusForbidden)
		case strings.Contains(err.Error(), "не авторизован"):
			util.HandleError(w, "не авторизован", http.StatusUnauthorized)
		default:
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", result.Document.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(result.Document.SizeBytes, 10))
		w.WriteHeader(http.StatusOK)
		return
	}

	// для продакшена можно оставить, чтобы сразу скачивать файл
	//if result.Document.IsFile {
	//	http.Redirect(w, r, result.GetURL, http.StatusFound)
	//	return
	//}

	resp := requestresponse.GetDocumentResponse{
		Data: requestresponse.GetDocumentData{
			Document: requestresponse.DocumentResponseFromModel(result.Document, result.GetURL),
		},
		ExpiresIn: strconv.Itoa(h.cfg.S3AndRedis),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetDocumentHead godoc
// @Summary Получение документа по ID
// @Description Возвращает документ в JSON или файл по ссылке.
// @Tags Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs/{doc_id} [head]
func (h *DocumentHandler) GetDocumentHead(w http.ResponseWriter, r *http.Request) {
	h.GetDocument(w, r)
}

// GetDocumentByToken godoc
// @Summary Получение документа по токену
// @Description Возвращает документ в JSON или файл по ссылке. Доступ к документу по его токену, авторизация не требуется.
// @Tags Documents
// @Accept json
// @Produce json
// @Param token path string true "Токен документа"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs/public/{token} [get]
func (h *DocumentHandler) GetDocumentByToken(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if token == "" {
		util.HandleError(w, "Токен документа обязателен", http.StatusBadRequest)
		return
	}

	result, err := h.DocumentService.GetDocumentByToken(r.Context(), token)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не удалось получить документ по токену"),
			strings.Contains(err.Error(), "документ не является публичным"),
			strings.Contains(err.Error(), "не найден"):
			util.HandleError(w, "документ не найден", http.StatusNotFound)
		default:
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", result.Document.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(result.Document.SizeBytes, 10))
		w.WriteHeader(http.StatusOK)
		return
	}

	//if result.Document.IsFile {
	//	http.Redirect(w, r, result.GetURL, http.StatusFound)
	//	return
	//}

	resp := requestresponse.GetDocumentResponse{
		Data: requestresponse.GetDocumentData{
			Document: requestresponse.DocumentResponseFromModel(result.Document, result.GetURL),
		},
		ExpiresIn: strconv.Itoa(h.cfg.S3AndRedis),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// GetDocumentByTokenHead godoc
// @Summary Получение документа по токену
// @Description Возвращает документ в JSON или файл по ссылке. Доступ к документу по его токену, авторизация не требуется.
// @Tags Documents
// @Accept json
// @Produce json
// @Param token path string true "Токен документа"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs/public/{token} [head]
func (h *DocumentHandler) GetDocumentByTokenHead(w http.ResponseWriter, r *http.Request) {
	h.GetDocumentByTokenHead(w, r)
}

// GetPublicDocumentByUUID godoc
// @Summary Получение публичного документа по UUID
// @Description Возвращает документ, если он публичный (is_public = true).
// @Tags Public Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 403 {object} requestresponse.ErrorResponse "Документ не публичный"
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /public/docs/{doc_id} [get]
func (h *DocumentHandler) GetPublicDocumentByUUID(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	docUUID := chi.URLParam(r, "doc_id")

	document, err := h.DocumentService.GetPublicDocument(ctx, docUUID, "")
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не удалось начать транзакцию"),
			strings.Contains(err.Error(), "публичный документ не найден"),
			strings.Contains(err.Error(), "не удалось закоммитить транзакцию"),
			strings.Contains(err.Error(), "не удалось сгенерировать pre-signed GET URL"):
			log.Println(err)
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", document.Document.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(document.Document.SizeBytes, 10))
		w.WriteHeader(http.StatusOK)
		return
	}

	if document == nil || document.Document.IsPublic == false {
		util.HandleError(w, "документ не найден или не публичный", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(document)
}

// GetPublicDocumentByUUIDHead godoc
// @Summary Получение публичного документа по UUID
// @Description Возвращает документ, если он публичный (is_public = true).
// @Tags Public Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 403 {object} requestresponse.ErrorResponse "Документ не публичный"
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /public/docs/{doc_id} [head]
func (h *DocumentHandler) GetPublicDocumentByUUIDHead(w http.ResponseWriter, r *http.Request) {
	h.GetPublicDocumentByUUID(w, r)
}

// GetPublicDocumentByToken godoc
// @Summary Получение публичного документа по токену
// @Description Возвращает документ, если он публичный (is_public = true).
// @Tags Public Documents
// @Accept json
// @Produce json
// @Param token path string true "Токен доступа к документу"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 403 {object} requestresponse.ErrorResponse "Документ не публичный"
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /public/docs/token/{token} [get]
func (h *DocumentHandler) GetPublicDocumentByToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	token := chi.URLParam(r, "token")

	if token == "" {
		util.HandleError(w, "токен документа обязателен", http.StatusBadRequest)
		return
	}

	document, err := h.DocumentService.GetPublicDocument(ctx, "", token)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не удалось начать транзакцию"),
			strings.Contains(err.Error(), "публичный документ не найден"),
			strings.Contains(err.Error(), "не удалось закоммитить транзакцию"),
			strings.Contains(err.Error(), "не удалось сгенерировать pre-signed GET URL"):
			log.Println(err)
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	if document == nil || document.Document.IsPublic == false {
		util.HandleError(w, "документ не найден или не публичный", http.StatusNotFound)
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("Content-Type", document.Document.MimeType)
		w.Header().Set("Content-Length", strconv.FormatInt(document.Document.SizeBytes, 10))
		w.WriteHeader(http.StatusOK)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(document)
}

// GetPublicDocumentByTokenHead godoc
// @Summary Получение публичного документа по токену
// @Description Возвращает документ, если он публичный (is_public = true).
// @Tags Public Documents
// @Accept json
// @Produce json
// @Param token path string true "Токен доступа к документу"
// @Success 200 {object} requestresponse.GetDocumentResponse
// @Failure 403 {object} requestresponse.ErrorResponse "Документ не публичный"
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /public/docs/token/{token} [head]
func (h *DocumentHandler) GetPublicDocumentByTokenHead(w http.ResponseWriter, r *http.Request) {
	h.GetPublicDocumentByToken(w, r)
}

// ShareDocument godoc
// @Summary Предоставление доступа к документу
// @Description Добавляет пользователя к документу для совместного доступа
// @Tags Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Param body body requestresponse.ShareDocumentRequest true "Тело запроса"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.ResponseMessage "Доступ успешно предоставлен"
// @Failure 400 {object} requestresponse.ErrorResponse "Некорректный запрос"
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован"
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещен"
// @Failure 404 {object} requestresponse.ErrorResponse "Документ не найден"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка"
// @Router /api/docs/{doc_id}/share [post]
func (h *DocumentHandler) ShareDocument(w http.ResponseWriter, r *http.Request) {
	docUUID := chi.URLParam(r, "doc_id")
	if docUUID == "" {
		util.HandleError(w, "ID документа обязателен", http.StatusBadRequest)
		return
	}

	var req requestresponse.ShareDocumentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.HandleError(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	claims, ok := r.Context().Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		util.HandleError(w, "Пользователь не авторизован", http.StatusUnauthorized)
		return
	}

	err := h.DocumentService.AddGrant(r.Context(), docUUID, claims.UserUUID, req.TargetUserUUID)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "доступ запрещён"):
			util.HandleError(w, "Доступ запрещен", http.StatusForbidden)
		case strings.Contains(err.Error(), "документ не найден"):
			util.HandleError(w, "Документ не найден", http.StatusNotFound)
		case strings.Contains(err.Error(), "[DocumentService]"):
			log.Println(err)
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	resp := requestresponse.ResponseMessage{Response: map[string]interface{}{docUUID: true}}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// RemoveGrantFromDocument godoc
// @Summary Удаление доступа к документу
// @Description Удаляет пользователя из списка доступа к документу (grant). Доступно только владельцу документа.
// @Tags Documents
// @Accept json
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Param body body requestresponse.RemoveGrantRequest true "Тело запроса, содержит UUID пользователя, которому нужно убрать доступ"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.ResponseMessage "Доступ успешно удален"
// @Failure 400 {object} requestresponse.ErrorResponse "Некорректный запрос"
// @Failure 401 {object} requestresponse.ErrorResponse "Пользователь не авторизован"
// @Failure 403 {object} requestresponse.ErrorResponse "Доступ запрещен"
// @Failure 404 {object} requestresponse.ErrorResponse "Документ не найден"
// @Failure 500 {object} requestresponse.ErrorResponse "Внутренняя ошибка сервера"
// @Router /api/docs/{doc_id}/remove-grant [post]
// @Security BearerAuth
func (h *DocumentHandler) RemoveGrantFromDocument(w http.ResponseWriter, r *http.Request) {
	docUUID := chi.URLParam(r, "doc_id")
	if docUUID == "" {
		util.HandleError(w, "ID документа обязателен", http.StatusBadRequest)
		return
	}

	var req requestresponse.RemoveGrantRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		util.HandleError(w, "Неверный формат запроса", http.StatusBadRequest)
		return
	}

	claims, ok := r.Context().Value(security.UserContextKey).(*security.Claims)
	if !ok || claims == nil {
		util.HandleError(w, "Пользователь не авторизован", http.StatusUnauthorized)
		return
	}

	err := h.DocumentService.RemoveGrant(r.Context(), docUUID, claims.UserUUID, req.TargetUserUUID)
	if err != nil {
		log.Println(err)
		switch err.Error() {
		case "[DocumentService] доступ запрещён: документ не принадлежит владельцу":
			util.HandleError(w, "Доступ запрещен", http.StatusForbidden)
		case "документ не найден":
			util.HandleError(w, "Документ не найден", http.StatusNotFound)
		default:
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		}
		return
	}

	resp := requestresponse.ResponseMessage{
		Response: map[string]interface{}{
			docUUID: true,
		},
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// DeleteDocument удаляет документ
// @Summary Удалить документ
// @Description Помечает документ как удаленный и удаляет файл из хранилища
// @Tags Documents
// @Produce json
// @Param doc_id path string true "UUID документа"
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.SuccessResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 403 {object} requestresponse.ErrorResponse
// @Failure 404 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs/{doc_id} [delete]
// @Security BearerAuth
func (h *DocumentHandler) DeleteDocument(w http.ResponseWriter, r *http.Request) {
	docUUID := chi.URLParam(r, "doc_id")
	if docUUID == "" {
		util.HandleError(w, "ID документа обязателен", http.StatusBadRequest)
		return
	}

	claims, ok := r.Context().Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		util.HandleError(w, "Пользователь не авторизован", http.StatusUnauthorized)
		return
	}

	response, err := h.DocumentService.DeleteDocument(r.Context(), docUUID, claims.UserUUID)
	if err != nil {
		log.Println(err)
		switch {
		case strings.Contains(err.Error(), "не найден"):
			util.HandleError(w, "Документ не найден", http.StatusNotFound)
		case strings.Contains(err.Error(), "только владелец"):
			util.HandleError(w, "Недостаточно прав для удаления", http.StatusForbidden)
		case strings.Contains(err.Error(), "S3"):
			log.Println(err)
			util.HandleError(w, "Ошибка при удалении файла", http.StatusInternalServerError)
		case strings.Contains(err.Error(), "[DocumentService]"):
			log.Println(err)
			util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		default:
			util.HandleError(w, "неизвестная ошибка", http.StatusInternalServerError)
		}
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// ListDocuments возвращает список документов пользователя
// @Summary Список документов
// @Description Возвращает список документов с фильтрацией и пагинацией
// @Tags Documents
// @Produce json
// @Param login query string false "Login пользователя для просмотра чужих документов"
// @Param key query string false "Имя колонки для фильтрации"
// @Param value query string false "Значение фильтра"
// @Param limit query int false "Лимит документов на странице" default(20) minimum(1) maximum(100)
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 {object} requestresponse.ListDocumentsResponse
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs [get]
// @Security BearerAuth
func (h *DocumentHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	claims, ok := r.Context().Value(security.UserContextKey).(*security.Claims)
	if ok == false || claims == nil {
		util.HandleError(w, "Пользователь не авторизован", http.StatusUnauthorized)
		return
	}

	login := r.URL.Query().Get("login") // если пусто — свои документы
	filterKey := r.URL.Query().Get("key")
	filterValue := r.URL.Query().Get("value")

	limit := 20
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		parsed, err := strconv.Atoi(limitStr)
		if err != nil || parsed < 1 {
			util.HandleError(w, "неверное значение limit", http.StatusBadRequest)
			return
		}
		if parsed > 100 {
			limit = 100
		} else {
			limit = parsed
		}
	}

	docs, nextCursor, err := h.DocumentService.ListDocuments(r.Context(), claims.UserUUID, login, filterKey, filterValue, limit)
	if err != nil {
		log.Println(err)
		util.HandleError(w, "внутренняя ошибка сервера", http.StatusInternalServerError)
		return
	}

	if r.Method == http.MethodHead {
		w.Header().Set("X-Total-Documents", strconv.Itoa(len(docs)))
		if nextCursor != "" {
			w.Header().Set("X-Next-Cursor", nextCursor)
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	data := struct {
		Docs []model.DocumentResponse `json:"docs"`
	}{
		Docs: docs,
	}

	resp := struct {
		Data       interface{} `json:"data"`
		NextCursor string      `json:"next_cursor,omitempty"`
		Count      int         `json:"count"`
	}{
		Data:       data,
		NextCursor: nextCursor,
		Count:      len(docs),
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// ListDocumentsHead возвращает заголовки списка документов (HEAD)
// @Summary Заголовки списка документов
// @Description Возвращает заголовки списка документов без тела (для HEAD запроса)
// @Tags Documents
// @Produce json
// @Param login query string false "Login пользователя для просмотра чужих документов"
// @Param key query string false "Имя колонки для фильтрации"
// @Param value query string false "Значение фильтра"
// @Param limit query int false "Лимит документов на странице" default(20) minimum(1) maximum(100)
// @Param Authorization header string true "Bearer токен" default(Bearer <access_token>)
// @Success 200 "Заголовки с информацией о документах"
// @Failure 400 {object} requestresponse.ErrorResponse
// @Failure 401 {object} requestresponse.ErrorResponse
// @Failure 500 {object} requestresponse.ErrorResponse
// @Router /api/docs [head]
// @Security BearerAuth
func (h *DocumentHandler) ListDocumentsHead(w http.ResponseWriter, r *http.Request) {
	h.ListDocuments(w, r)
}
