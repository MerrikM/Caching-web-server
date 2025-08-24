package service

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/ports"
	"caching-web-server/internal/security"
	"caching-web-server/internal/util"
	"context"
	"errors"
	"fmt"
	_ "github.com/aws/aws-sdk-go-v2/config"
	"log"
	"time"
)

type DocumentService struct {
	documentRepository ports.DocumentRepository
	cacheRepository    ports.CacheRepository
	grantRepository    ports.GrantDocumentRepository
	storageInterface   ports.S3Storage
	userRepository     ports.UserRepository
	ttl                time.Duration
}

func NewDocumentService(
	documentRepository ports.DocumentRepository,
	cacheRepository ports.CacheRepository,
	shareRepository ports.GrantDocumentRepository,
	storageInterface ports.S3Storage,
	userRepository ports.UserRepository,
	ttl time.Duration,
) *DocumentService {
	return &DocumentService{
		documentRepository: documentRepository,
		cacheRepository:    cacheRepository,
		grantRepository:    shareRepository,
		storageInterface:   storageInterface,
		userRepository:     userRepository,
		ttl:                ttl,
	}
}

// CreateDocument : создаёт документ, возвращает pre-signed PUT URL для загрузки и кеширует метаданные
func (s *DocumentService) CreateDocument(ctx context.Context, document *model.Document) (string, error) {
	db, ok := ctx.Value("db").(*config.Database)
	if ok == false {
		return "", fmt.Errorf("[DocumentService] database connection не найден в context")
	}

	putURL, err := s.storageInterface.GeneratePresignedPutURL(ctx, document.StoragePath, s.ttl)
	if err != nil {
		return "", util.LogError("[DocumentService] не удалось URL", err)
	}

	if err := s.documentRepository.Create(ctx, db, document); err != nil {
		return "", util.LogError("[DocumentService] не удалось сохранить документ в БД", err)
	}

	log.Printf("[DocumentService] документ %s успешно создан", document.FilenameOriginal)

	return putURL, nil
}

// GetDocumentByUUID : возвращает документ для авторизованного пользователя (владелец или по grants)
func (s *DocumentService) GetDocumentByUUID(ctx context.Context, documentUUID string) (*model.GetDocumentResult, error) {
	var document *model.Document
	var err error

	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return nil, fmt.Errorf("[DocumentService] database connection не найден в context")
	}

	claims, err := security.GetClaimsFromContext(ctx)
	if err != nil || claims == nil {
		return nil, fmt.Errorf("[DocumentService] пользователь не авторизован")
	}

	document, err = s.cacheRepository.GetDocument(ctx, documentUUID)
	if err != nil {
		log.Printf("[DocumentService] ошибка кэширования: %v", err)
	}

	if document == nil {
		exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
		if err != nil {
			return nil, util.LogError("[DocumentService] не удалось начать транзакцию", err)
		}
		defer rollback()

		document, _, err = s.documentRepository.GetByUUID(ctx, exec, documentUUID, claims.UserUUID)
		if err != nil {
			return nil, util.LogError("[DocumentService] документ не найден или доступ запрещён", err)
		}

		grants, err := s.grantRepository.ListGrants(ctx, exec, documentUUID)
		if err != nil {
			return nil, util.LogError("[DocumentService] не удалось получить список grant", err)
		}
		document.GrantLogins = grants

		if document.OwnerUUID != claims.UserUUID && !document.IsPublic {
			hasAccess, err := s.grantRepository.HasAccess(ctx, exec, documentUUID, claims.UserUUID)
			if err != nil {
				return nil, util.LogError("[DocumentService] ошибка проверки доступа", err)
			}
			if !hasAccess {
				return nil, fmt.Errorf("[DocumentService] доступ запрещён")
			}
		}

		if err := commit(); err != nil {
			return nil, util.LogError("[DocumentService] не удалось закоммитить транзакцию", err)
		}

		if err := s.cacheRepository.SetDocument(ctx, document); err != nil {
			fmt.Printf("[DocumentService] ошибка кэширования документа: %v\n", err)
		}

		log.Printf("[DocumentService] документ %s взят из БД и успешно кэширован Redis", document.FilenameOriginal)
	} else {
		if document.IsPublic == false && document.OwnerUUID != claims.UserUUID {
			hasAccess, err := s.grantRepository.HasAccess(ctx, db, documentUUID, claims.UserUUID)
			if err != nil {
				return nil, util.LogError("[DocumentService] ошибка проверки доступа", err)
			}
			if !hasAccess {
				return nil, fmt.Errorf("[DocumentService] доступ запрещён")
			}
		}
		log.Printf("[DocumentService] документ %s взят из кэша Redis", document.FilenameOriginal)
	}

	var getURL string
	if document.StoragePath != "" {
		getURL, err = s.storageInterface.GeneratePresignedGetURL(ctx, document.StoragePath, s.ttl)
		if err != nil {
			return nil, util.LogError("[DocumentService] не удалось сгенерировать pre-signed GET URL", err)
		}
	}

	return &model.GetDocumentResult{
		Document: document,
		GetURL:   getURL,
	}, nil
}

// GetDocumentByToken : возвращает публичный документ по токену
func (s *DocumentService) GetDocumentByToken(ctx context.Context, token string) (*model.GetDocumentResult, error) {
	db, ok := ctx.Value("db").(*config.Database)
	if ok == false {
		return nil, fmt.Errorf("[UserService] database connection не найден в context")
	}

	document, err := s.documentRepository.GetByToken(ctx, db, token)
	if err != nil {
		return nil, util.LogError("[DocumentService] не удалось получить документ по токену", err)
	}

	if document == nil || document.IsPublic == false {
		return nil, errors.New("[DocumentService] документ не является публичным или не найден")
	}

	var getURL string
	if document.StoragePath != "" {
		getURL, err = s.storageInterface.GeneratePresignedGetURL(ctx, document.StoragePath, s.ttl)
		if err != nil {
			return nil, util.LogError("[DocumentService] не удалось сгенерировать pre-signed GET URL", err)
		}
	}

	return &model.GetDocumentResult{
		Document: document,
		GetURL:   getURL,
	}, nil
}

// GetPublicDocument : возвращает публичный документ по UUID или токену
func (s *DocumentService) GetPublicDocument(ctx context.Context, documentUUID, token string) (*model.GetDocumentResult, error) {
	var document *model.Document
	var err error

	exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
	if err != nil {
		return nil, util.LogError("[DocumentService] не удалось начать транзакцию", err)
	}
	defer rollback()

	if token != "" {
		document, err = s.documentRepository.GetPublicByToken(ctx, exec, token)
	} else {
		document, err = s.documentRepository.GetPublicByUUID(ctx, exec, documentUUID)
	}
	if err != nil {
		return nil, util.LogError("[DocumentService] публичный документ не найден", err)
	}

	if err := commit(); err != nil {
		return nil, util.LogError("[DocumentService] не удалось закоммитить транзакцию", err)
	}

	// генерируем ссылку
	var getURL string
	if document != nil && document.StoragePath != "" {
		getURL, err = s.storageInterface.GeneratePresignedGetURL(ctx, document.StoragePath, s.ttl)
		if err != nil {
			return nil, util.LogError("[DocumentService] не удалось сгенерировать pre-signed GET URL", err)
		}
	}

	return &model.GetDocumentResult{
		Document: document,
		GetURL:   getURL,
	}, nil
}

// ShareDocument : добавить пользователя к документу
func (s *DocumentService) ShareDocument(
	ctx context.Context,
	documentUUID string,
	ownerUUID string,
	targetUserUUID string,
) error {
	exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
	if err != nil {
		return util.LogError("[DocumentService] не удалось начать транзакцию", err)
	}
	defer rollback()

	document, _, err := s.documentRepository.GetByUUID(ctx, exec, documentUUID, ownerUUID)
	if err != nil {
		return util.LogError("[DocumentService] документ не найден", err)
	}

	if document.OwnerUUID != ownerUUID {
		return util.LogError("[DocumentService] вы не являетесь владельцем документа", err)
	}

	exists, err := s.userRepository.Exists(ctx, exec, ownerUUID)
	if err != nil {
		return util.LogError("[DocumentService] ошибка проверки владельца", err)
	}
	if exists == false {
		return fmt.Errorf("[DocumentService] владелец документа не найден")
	}

	exists, err = s.userRepository.Exists(ctx, exec, targetUserUUID)
	if err != nil {
		return util.LogError("[DocumentService] ошибка проверки пользователя", err)
	}
	if exists == false {
		return fmt.Errorf("[DocumentService] пользователь для шаринга не найден")
	}

	if err := s.grantRepository.AddGrant(ctx, exec, documentUUID, ownerUUID, targetUserUUID); err != nil {
		return util.LogError("[DocumentService] ошибка изменения прав доступа", err)
	}

	if err := commit(); err != nil {
		return util.LogError("[DocumentService] ошибка коммита транзакции", err)
	}

	// удаляем документ из кэша (чтобы потом перечитать актуальные grant при получении документа)
	if err := s.cacheRepository.DeleteDocument(ctx, documentUUID); err != nil {
		fmt.Printf("[DocumentService] ошибка удаления документа из кэша: %v", err)
	}

	return nil
}

// DeleteDocument помечает документ удалённым, инвалидирует кэш и удаляет файл из S3
func (s *DocumentService) DeleteDocument(ctx context.Context, documentUUID string, userUUID string) (map[string]bool, error) {
	exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
	if err != nil {
		return nil, util.LogError("[DocumentService] ошибка начала транзакции", err)
	}
	defer rollback()

	document, _, err := s.documentRepository.GetByUUID(ctx, exec, documentUUID, userUUID)
	if err != nil {
		return nil, util.LogError("[DocumentService] документ не найден", err)
	}

	if document.OwnerUUID != userUUID {
		return nil, fmt.Errorf("[DocumentService] только владелец может удалить документ")
	}

	deletedUUID, err := s.documentRepository.Delete(ctx, exec, documentUUID, document.OwnerUUID)
	if err != nil {
		return nil, util.LogError("[DocumentService] ошибка удаления документа из БД", err)
	}

	if err := commit(); err != nil {
		return nil, fmt.Errorf("[DocumentService] ошибка коммита транзакции: %w", err)
	}

	if err := s.cacheRepository.DeleteDocument(ctx, documentUUID); err != nil {
		fmt.Printf("[DocumentService] ошибка удаления из кэша: %v\n", err)
	}

	if err := s.storageInterface.DeleteObject(ctx, document.StoragePath); err != nil {
		return nil, util.LogError("[DocumentService] ошибка удаления файла из S3", err)
	}

	log.Printf("[DocumentService] документ %s успешно удален", document.FilenameOriginal)

	response := map[string]bool{
		deletedUUID: true,
	}

	return response, nil
}

// ListDocuments : список документов владельца (cursor-based pagination) с pre-signed URL
func (s *DocumentService) ListDocuments(ctx context.Context, userUUID string, login string, filterKey string, filterValue string, limit int) ([]model.DocumentResponse, string, error) {
	db, ok := ctx.Value("db").(*config.Database)
	if !ok {
		return nil, "", fmt.Errorf("[DocumentService] database connection не найден в context")
	}

	docs, err := s.documentRepository.ListDocuments(ctx, db, userUUID, login, filterKey, filterValue, limit)
	if err != nil {
		return nil, "", util.LogError("[DocumentService] не удалось получить список документов", err)
	}

	responses := make([]model.DocumentResponse, 0, len(docs))

	for _, doc := range docs {
		grants, err := s.grantRepository.ListGrants(ctx, db, doc.UUID)
		if err != nil {
			fmt.Printf("[DocumentService] не удалось получить grants для документа %s: %v\n", doc.UUID, err)
			grants = []string{} // на случай ошибки оставляем пустой массив
		}

		url, err := s.storageInterface.GeneratePresignedGetURL(ctx, doc.StoragePath, 15*time.Minute)
		if err != nil {
			fmt.Printf("[DocumentService] ошибка генерации pre-signed URL для документа %s: %v\n", doc.UUID, err)
			url = ""
		}

		responses = append(responses, model.DocumentResponse{
			UUID:         doc.UUID,
			Title:        doc.FilenameOriginal,
			PresignedURL: url,
			File:         doc.IsFile,
			IsPublic:     doc.IsPublic,
			GrantLogins:  grants,
			MimeType:     doc.MimeType,
			CreatedAt:    doc.CreatedAt,
		})
	}

	var nextCursor string
	if len(docs) == limit {
		nextCursor = docs[len(docs)-1].UUID
	}

	return responses, nextCursor, nil
}

// AddGrant : добавляет пользователя к документу для совместного доступа и инвалидирует кэш
func (s *DocumentService) AddGrant(ctx context.Context, documentUUID, ownerUUID, targetUserUUID string) error {
	exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
	if err != nil {
		return util.LogError("[DocumentService] ошибка начала транзакции", err)
	}
	defer rollback()

	exists, err := s.grantRepository.CheckOwner(ctx, exec, documentUUID, ownerUUID)
	if err != nil {
		return err
	}
	if exists == false {
		return fmt.Errorf("[DocumentService] доступ запрещён: документ не принадлежит владельцу")
	}

	if err := s.grantRepository.AddGrant(ctx, exec, documentUUID, ownerUUID, targetUserUUID); err != nil {
		return util.LogError("[DocumentService] не удалось добавить доступ к документу", err)
	}

	if err := commit(); err != nil {
		return util.LogError("[DocumentService] ошибка коммита транзакции", err)
	}

	// Инвалидируем кэш документа, чтобы новые гранты были учтены
	if err := s.cacheRepository.DeleteDocument(ctx, documentUUID); err != nil {
		fmt.Printf("[DocumentService] ошибка удаления документа из кэша: %v\n", err)
	}

	return nil
}

// RemoveGrant : удаляет пользователя из доступа к документу и инвалидирует кэш
func (s *DocumentService) RemoveGrant(ctx context.Context, documentUUID, ownerUUID, targetUserUUID string) error {
	exec, rollback, commit, err := s.documentRepository.BeginTX(ctx)
	if err != nil {
		return util.LogError("[DocumentService] ошибка начала транзакции", err)
	}
	defer rollback()

	exists, err := s.grantRepository.CheckOwner(ctx, exec, documentUUID, ownerUUID)
	if err != nil {
		return err
	}
	if exists == false {
		return fmt.Errorf("[DocumentService] доступ запрещён: документ не принадлежит владельцу")
	}

	if err := s.grantRepository.RemoveGrant(ctx, exec, documentUUID, targetUserUUID); err != nil {
		return err
	}

	if err := commit(); err != nil {
		return fmt.Errorf("[DocumentService] ошибка коммита транзакции: %w", err)
	}

	if err := s.cacheRepository.DeleteDocument(ctx, documentUUID); err != nil {
		fmt.Printf("[DocumentService] ошибка удаления документа из кэша: %v\n", err)
	}

	return nil
}
