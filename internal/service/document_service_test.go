package service_test

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/security"
	"caching-web-server/internal/service"
	_ "caching-web-server/internal/service"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type contextKey string

const (
	ContextKeyDB       contextKey = "db"
	ContextKeyUserUUID contextKey = "userUUID"
)

type MockDocumentRepository struct{ mock.Mock }

func (m *MockDocumentRepository) Create(ctx context.Context, exec sqlx.ExtContext, doc *model.Document) error {
	return m.Called(ctx, exec, doc).Error(0)
}

// Заглушки для остальных методов
func (m *MockDocumentRepository) GetByUUID(ctx context.Context, exec sqlx.ExtContext, documentUUID string, userID string) (*model.Document, []string, error) {
	args := m.Called(ctx, exec, documentUUID, userID)
	doc := args.Get(0)
	if doc == nil {
		return nil, nil, args.Error(1)
	}
	return doc.(*model.Document), args.Get(1).([]string), args.Error(2)
}

func (m *MockDocumentRepository) GetByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error) {
	args := m.Called(ctx, exec, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Document), args.Error(1)
}

func (m *MockDocumentRepository) GetPublicByUUID(ctx context.Context, exec sqlx.ExtContext, uuid string) (*model.Document, error) {
	args := m.Called(ctx, exec, uuid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Document), args.Error(1)
}

func (m *MockDocumentRepository) GetPublicByToken(ctx context.Context, exec sqlx.ExtContext, token string) (*model.Document, error) {
	args := m.Called(ctx, exec, token)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Document), args.Error(1)
}

func (m *MockDocumentRepository) ListDocuments(ctx context.Context, exec sqlx.ExtContext, ownerUUID, login, filterKey, filterValue string, limit int) ([]model.Document, error) {
	args := m.Called(ctx, exec, ownerUUID, login, filterKey, filterValue, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]model.Document), args.Error(1)
}

func (m *MockDocumentRepository) Delete(ctx context.Context, exec sqlx.ExtContext, docID string, ownerUUID string) (string, error) {
	args := m.Called(ctx, exec, docID, ownerUUID)
	return args.String(0), args.Error(1)
}

func (m *MockDocumentRepository) BeginTX(ctx context.Context) (sqlx.ExtContext, func() error, func() error, error) {
	args := m.Called(ctx)
	return args.Get(0).(sqlx.ExtContext), args.Get(1).(func() error), args.Get(2).(func() error), args.Error(3)
}

func (m *MockCacheRepository) GetDocument(ctx context.Context, uuid string) (*model.Document, error) {
	args := m.Called(ctx, uuid)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Document), args.Error(1)
}

func (m *MockCacheRepository) DeleteDocument(ctx context.Context, uuid string) error {
	args := m.Called(
		mock.Anything, // любой context.Context
		uuid,
	)
	return args.Error(0)
}

func (m *MockS3Storage) GeneratePresignedGetURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	args := m.Called(ctx, key, expire)
	return args.String(0), args.Error(1)
}

func (m *MockS3Storage) DeleteObject(ctx context.Context, key string) error {
	return m.Called(ctx, key).Error(0)
}

func (m *MockGrantRepository) AddGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID string, ownerUUID string, targetUserUUID string) error {
	args := m.Called(ctx, exec, documentUUID, ownerUUID, targetUserUUID)
	return args.Error(0)
}

func (m *MockGrantRepository) RemoveGrant(ctx context.Context, exec sqlx.ExtContext, documentUUID string, userUUID string) error {
	args := m.Called(ctx, exec, documentUUID, userUUID)
	return args.Error(0)
}

type MockS3Storage struct{ mock.Mock }

func (m *MockS3Storage) GeneratePresignedPutURL(ctx context.Context, key string, ttl time.Duration) (string, error) {
	args := m.Called(ctx, key, ttl)
	return args.String(0), args.Error(1)
}

type MockCacheRepository struct {
	mock.Mock
}

func (m *MockCacheRepository) SetDocument(ctx context.Context, doc *model.Document) error {
	return m.Called(ctx, doc).Error(0)
}

type fakeTx struct{}

func (f *fakeTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return nil, nil
}
func (f *fakeTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return nil, nil
}
func (f *fakeTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return &sql.Row{}
}
func (f *fakeTx) QueryxContext(ctx context.Context, query string, args ...interface{}) (*sqlx.Rows, error) {
	return nil, nil
}
func (f *fakeTx) QueryRowxContext(ctx context.Context, query string, args ...interface{}) *sqlx.Row {
	return &sqlx.Row{}
}
func (f *fakeTx) BindNamed(query string, arg interface{}) (string, []interface{}, error) {
	return "", nil, nil
}
func (f *fakeTx) DriverName() string         { return "fake" }
func (f *fakeTx) Commit() error              { return nil }
func (f *fakeTx) Rollback() error            { return nil }
func (f *fakeTx) Rebind(query string) string { return query }

// ===== Функция для создания сервиса с моками =====
func newTestDocumentService() (*service.DocumentService, *MockDocumentRepository, *MockS3Storage, *MockCacheRepository) {
	mockDocRepo := new(MockDocumentRepository)
	mockStorage := new(MockS3Storage)
	mockCache := new(MockCacheRepository)

	svc := service.NewDocumentService(
		mockDocRepo,
		mockCache,
		nil, // GrantRepository не нужен для CreateDocument
		mockStorage,
		nil,       // UserRepository не нужен для CreateDocument
		time.Hour, // TTL
	)

	return svc, mockDocRepo, mockStorage, mockCache
}

// ===== Тесты CreateDocument =====

func TestCreateDocument_Success(t *testing.T) {
	svc, mockDocRepo, mockStorage, _ := newTestDocumentService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})
	doc := &model.Document{
		UUID:             "doc1",
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Hour
	mockStorage.On("GeneratePresignedPutURL", ctx, doc.StoragePath, ttl).Return("http://put-url", nil)
	mockDocRepo.On("Create", ctx, mock.Anything, doc).Return(nil)

	putURL, err := svc.CreateDocument(ctx, doc)

	assert.NoError(t, err)
	assert.Equal(t, "http://put-url", putURL)
	mockStorage.AssertExpectations(t)
	mockDocRepo.AssertExpectations(t)
}

func TestCreateDocument_StorageError(t *testing.T) {
	svc, _, mockStorage, _ := newTestDocumentService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})
	doc := &model.Document{
		UUID:             "doc1",
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Hour
	mockStorage.On("GeneratePresignedPutURL", ctx, doc.StoragePath, ttl).Return("", errors.New("s3 error"))

	putURL, err := svc.CreateDocument(ctx, doc)

	assert.Error(t, err)
	assert.Equal(t, "", putURL)
}

func TestCreateDocument_RepositoryError(t *testing.T) {
	svc, mockDocRepo, mockStorage, _ := newTestDocumentService()
	ctx := context.WithValue(context.Background(), "db", &config.Database{})
	doc := &model.Document{
		UUID:             "doc1",
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Hour
	mockStorage.On("GeneratePresignedPutURL", ctx, doc.StoragePath, ttl).Return("http://put-url", nil)
	mockDocRepo.On("Create", ctx, mock.Anything, doc).Return(errors.New("db error"))

	putURL, err := svc.CreateDocument(ctx, doc)

	assert.Error(t, err)
	assert.Equal(t, "", putURL)
}

// ===== Тестируем GetDocumentByUUID =====

func newTestDocumentServiceWithGrants() (*service.DocumentService, *MockDocumentRepository, *MockS3Storage, *MockCacheRepository, *MockGrantRepository) {
	mockDocRepo := new(MockDocumentRepository)
	mockStorage := new(MockS3Storage)
	mockCache := new(MockCacheRepository)
	mockGrantRepo := new(MockGrantRepository)

	svc := service.NewDocumentService(
		mockDocRepo,
		mockCache,
		mockGrantRepo,
		mockStorage,
		nil,
		time.Minute,
	)

	return svc, mockDocRepo, mockStorage, mockCache, mockGrantRepo
}

// Мок GrantRepository
type MockGrantRepository struct{ mock.Mock }

func (m *MockGrantRepository) ListGrants(ctx context.Context, exec sqlx.ExtContext, documentUUID string) ([]string, error) {
	args := m.Called(ctx, exec, documentUUID)
	return args.Get(0).([]string), args.Error(1)
}
func (m *MockGrantRepository) CheckOwner(ctx context.Context, exec sqlx.ExtContext, documentUUID, ownerUUID string) (bool, error) {
	args := m.Called(ctx, exec, documentUUID, ownerUUID)
	return args.Bool(0), args.Error(1)
}
func (m *MockGrantRepository) HasAccess(ctx context.Context, exec sqlx.ExtContext, documentUUID, userUUID string) (bool, error) {
	args := m.Called(ctx, exec, documentUUID, userUUID)
	return args.Bool(0), args.Error(1)
}

// ===== Тесты =====

func TestGetDocumentByUUID_FromCache(t *testing.T) {
	svc, _, mockStorage, mockCache, _ := newTestDocumentServiceWithGrants()

	ctx := context.WithValue(context.Background(), security.UserContextKey, &security.Claims{
		UserUUID: "user1",
	})
	ctx = context.WithValue(ctx, "db", &config.Database{})

	doc := &model.Document{
		UUID:             "doc1",
		OwnerUUID:        "user1",
		IsPublic:         false,
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Minute

	mockCache.On("GetDocument", ctx, "doc1").Return(doc, nil)
	mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, ttl).Return("http://get-url", nil)

	res, err := svc.GetDocumentByUUID(ctx, "doc1")

	assert.NoError(t, err)
	assert.Equal(t, "http://get-url", res.GetURL)
	assert.Equal(t, doc, res.Document)

	mockCache.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
}

func TestGetDocumentByUUID_WithMocks(t *testing.T) {
	svc, mockDocRepo, mockStorage, mockCache, mockGrantRepo := newTestDocumentServiceWithGrants()

	ctx := context.Background()
	ctx = context.WithValue(ctx, security.UserContextKey, &security.Claims{
		UserUUID: "user1",
	})
	ctx = context.WithValue(ctx, "db", &config.Database{})

	doc := &model.Document{
		UUID:             "doc1",
		OwnerUUID:        "user1",
		IsPublic:         false,
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	mockTx := &fakeTx{}
	mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil).Once()
	mockDocRepo.On("GetByUUID", ctx, mockTx, "doc1", "user1").Return(doc, []string{}, nil).Once()
	mockGrantRepo.On("ListGrants", ctx, mockTx, "doc1").Return([]string{"user2"}, nil).Once()
	mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, time.Minute).Return("http://get-url", nil).Once()
	mockCache.On("SetDocument", ctx, doc).Return(nil).Once()
	mockCache.On("GetDocument", ctx, "doc1").Return(nil, nil).Once()
	mockGrantRepo.On("HasAccess", ctx, mock.Anything, "doc1", "user1").Return(true, nil).Maybe()

	res, err := svc.GetDocumentByUUID(ctx, "doc1")
	require.NoError(t, err)
	assert.Equal(t, doc, res.Document)
	assert.Equal(t, "http://get-url", res.GetURL)

	mockDocRepo.AssertExpectations(t)
	mockGrantRepo.AssertExpectations(t)
	mockStorage.AssertExpectations(t)
	mockCache.AssertExpectations(t)
}

func TestGetDocumentByUUID(t *testing.T) {
	svc, mockDocRepo, mockStorage, mockCache, mockGrantRepo := newTestDocumentServiceWithGrants()
	ctx := context.Background()
	ctx = context.WithValue(ctx, security.UserContextKey, &security.Claims{
		UserUUID: "user1",
	})
	ctx = context.WithValue(ctx, "db", &config.Database{})

	docDB := &model.Document{
		UUID:             "doc1",
		OwnerUUID:        "user1",
		IsPublic:         false,
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Minute

	tests := []struct {
		name           string
		setupMocks     func()
		expectedErr    bool
		expectedGetURL string
		expectedDoc    *model.Document
	}{
		{
			name: "Document in cache",
			setupMocks: func() {
				mockCache.On("GetDocument", ctx, "doc1").Return(docDB, nil).Once()
				mockStorage.On("GeneratePresignedGetURL", ctx, docDB.StoragePath, ttl).Return("http://get-url", nil).Once()
			},
			expectedErr:    false,
			expectedGetURL: "http://get-url",
			expectedDoc:    docDB,
		},
		{
			name: "Document from DB, public",
			setupMocks: func() {
				publicDoc := &model.Document{
					UUID:             "doc1",
					OwnerUUID:        "user1",
					IsPublic:         true,
					FilenameOriginal: "file.txt",
					StoragePath:      "docs/doc1.txt",
					GrantLogins:      []string{}, // Устанавливаем пустой срез
				}
				mockCache.On("GetDocument", ctx, "doc1").Return(nil, nil).Once()
				mockTx := &fakeTx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil).Once()
				mockDocRepo.On("GetByUUID", ctx, mockTx, "doc1", "user1").Return(publicDoc, []string{}, nil).Once()
				mockGrantRepo.On("ListGrants", ctx, mockTx, "doc1").Return([]string{}, nil).Once()
				mockStorage.On("GeneratePresignedGetURL", ctx, publicDoc.StoragePath, ttl).Return("http://get-url", nil).Once()
				mockCache.On("SetDocument", ctx, publicDoc).Return(nil).Once()
			},
			expectedErr:    false,
			expectedGetURL: "http://get-url",
			expectedDoc: &model.Document{
				UUID:             "doc1",
				OwnerUUID:        "user1",
				IsPublic:         true,
				FilenameOriginal: "file.txt",
				StoragePath:      "docs/doc1.txt",
				GrantLogins:      []string{}, // Исправляем на пустой срез
			},
		},
		{
			name: "Document from DB, private, no access",
			setupMocks: func() {
				privateDoc := &model.Document{
					UUID:             "doc1",
					OwnerUUID:        "user2",
					IsPublic:         false,
					FilenameOriginal: "file.txt",
					StoragePath:      "docs/doc1.txt",
				}
				mockCache.On("GetDocument", ctx, "doc1").Return(nil, nil).Once()
				mockTx := &fakeTx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil).Once()
				mockDocRepo.On("GetByUUID", ctx, mockTx, "doc1", "user1").Return(privateDoc, []string{}, nil).Once()
				mockGrantRepo.On("ListGrants", ctx, mockTx, "doc1").Return([]string{}, nil).Once()
				mockGrantRepo.On("HasAccess", ctx, mockTx, "doc1", "user1").Return(false, nil).Once()
			},
			expectedErr: true,
		},
		{
			name: "Error generating pre-signed URL",
			setupMocks: func() {
				mockCache.On("GetDocument", ctx, "doc1").Return(docDB, nil).Once()
				mockStorage.On("GeneratePresignedGetURL", ctx, docDB.StoragePath, ttl).Return("", fmt.Errorf("failed")).Once()
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockCache.ExpectedCalls = nil
			mockDocRepo.ExpectedCalls = nil
			mockGrantRepo.ExpectedCalls = nil
			mockStorage.ExpectedCalls = nil

			tt.setupMocks()

			res, err := svc.GetDocumentByUUID(ctx, "doc1")
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedDoc, res.Document)
			assert.Equal(t, tt.expectedGetURL, res.GetURL)

			mockCache.AssertExpectations(t)
			mockDocRepo.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
			mockStorage.AssertExpectations(t)
		})
	}
}

func TestGetDocumentByToken(t *testing.T) {
	svc, mockDocRepo, mockStorage, mockCache, mockGrantRepo := newTestDocumentServiceWithGrants()
	ctx := context.Background()
	ctx = context.WithValue(ctx, "db", &config.Database{})

	doc := &model.Document{
		UUID:             "doc1",
		IsPublic:         true,
		FilenameOriginal: "file.txt",
		StoragePath:      "docs/doc1.txt",
	}

	ttl := time.Minute

	tests := []struct {
		name           string
		ctx            context.Context
		token          string
		setupMocks     func()
		expectedErr    bool
		expectedErrMsg string
		expectedDoc    *model.Document
		expectedGetURL string
	}{
		{
			name:  "Success",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(doc, nil).Once()
				mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, ttl).Return("http://get-url", nil).Once()
			},
			expectedErr:    false,
			expectedDoc:    doc,
			expectedGetURL: "http://get-url",
		},
		{
			name:  "Document not found",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(nil, errors.New("not found")).Once()
			},
			expectedErr:    true,
			expectedErrMsg: "не удалось получить документ по токену: not found",
		},
		{
			name:  "Document not public",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				privateDoc := &model.Document{
					UUID:             "doc1",
					IsPublic:         false,
					FilenameOriginal: "file.txt",
					StoragePath:      "docs/doc1.txt",
				}
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(privateDoc, nil).Once()
			},
			expectedErr:    true,
			expectedErrMsg: "документ не является публичным или не найден",
		},
		{
			name:  "Document nil",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(nil, nil).Once()
			},
			expectedErr:    true,
			expectedErrMsg: "документ не является публичным или не найден",
		},
		{
			name:  "No database in context",
			ctx:   context.Background(), // Контекст без "db"
			token: "token1",
			setupMocks: func() {
				// Нет моков, так как метод не доходит до вызова репозитория
			},
			expectedErr:    true,
			expectedErrMsg: "database connection не найден в context",
		},
		{
			name:  "Error generating pre-signed URL",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(doc, nil).Once()
				mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, ttl).Return("", errors.New("failed")).Once()
			},
			expectedErr:    true,
			expectedErrMsg: "не удалось сгенерировать pre-signed GET URL: failed",
		},
		{
			name:  "Empty StoragePath",
			ctx:   ctx,
			token: "token1",
			setupMocks: func() {
				emptyPathDoc := &model.Document{
					UUID:             "doc1",
					IsPublic:         true,
					FilenameOriginal: "file.txt",
					StoragePath:      "",
				}
				mockDocRepo.On("GetByToken", ctx, mock.Anything, "token1").Return(emptyPathDoc, nil).Once()
			},
			expectedErr: false,
			expectedDoc: &model.Document{
				UUID:             "doc1",
				IsPublic:         true,
				FilenameOriginal: "file.txt",
				StoragePath:      "",
			},
			expectedGetURL: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocRepo.ExpectedCalls = nil
			mockStorage.ExpectedCalls = nil
			mockCache.ExpectedCalls = nil
			mockGrantRepo.ExpectedCalls = nil

			tt.setupMocks()

			res, err := svc.GetDocumentByToken(tt.ctx, tt.token)
			if tt.expectedErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectedErrMsg)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedDoc, res.Document)
			assert.Equal(t, tt.expectedGetURL, res.GetURL)

			mockDocRepo.AssertExpectations(t)
			mockStorage.AssertExpectations(t)
			mockCache.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
		})
	}
}

func TestGetPublicDocument(t *testing.T) {
	svc, mockDocRepo, mockStorage, _, _ := newTestDocumentServiceWithGrants()
	ctx := context.Background()

	doc := &model.Document{
		UUID:        "doc1",
		StoragePath: "docs/doc1.txt",
		IsPublic:    true,
	}

	ttl := time.Minute

	tests := []struct {
		name           string
		documentUUID   string
		token          string
		setupMocks     func()
		expectedErr    bool
		expectedGetURL string
		expectedDoc    *model.Document
	}{
		{
			name:         "Get document by UUID",
			documentUUID: "doc1",
			token:        "",
			setupMocks: func() {
				mockTx := &sqlx.Tx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil)
				mockDocRepo.On("GetPublicByUUID", ctx, mockTx, "doc1").Return(doc, nil)
				mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, ttl).Return("http://get-url", nil)
			},
			expectedErr:    false,
			expectedGetURL: "http://get-url",
			expectedDoc:    doc,
		},
		{
			name:  "Get document by token",
			token: "token123",
			setupMocks: func() {
				mockTx := &sqlx.Tx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil)
				mockDocRepo.On("GetPublicByToken", ctx, mockTx, "token123").Return(doc, nil)
				mockStorage.On("GeneratePresignedGetURL", ctx, doc.StoragePath, ttl).Return("http://get-url", nil)
			},
			expectedErr:    false,
			expectedGetURL: "http://get-url",
			expectedDoc:    doc,
		},
		{
			name:         "Document not found",
			documentUUID: "doc2",
			setupMocks: func() {
				mockTx := &sqlx.Tx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil)
				mockDocRepo.On("GetPublicByUUID", ctx, mockTx, "doc2").Return(nil, fmt.Errorf("not found"))
			},
			expectedErr: true,
		},
		{
			name:         "Error generating pre-signed URL",
			documentUUID: "doc3",
			setupMocks: func() {
				doc3 := &model.Document{UUID: "doc3", StoragePath: "docs/doc3.txt"}
				mockTx := &sqlx.Tx{}
				mockDocRepo.On("BeginTX", ctx).Return(mockTx, func() error { return nil }, func() error { return nil }, nil)
				mockDocRepo.On("GetPublicByUUID", ctx, mockTx, "doc3").Return(doc3, nil)
				mockStorage.On("GeneratePresignedGetURL", ctx, doc3.StoragePath, ttl).Return("", fmt.Errorf("failed"))
			},
			expectedErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocRepo.ExpectedCalls = nil
			mockStorage.ExpectedCalls = nil

			tt.setupMocks()

			res, err := svc.GetPublicDocument(ctx, tt.documentUUID, tt.token)
			if tt.expectedErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.expectedDoc, res.Document)
			assert.Equal(t, tt.expectedGetURL, res.GetURL)

			mockDocRepo.AssertExpectations(t)
			mockStorage.AssertExpectations(t)
		})
	}
}

func TestShareDocument_AllCases(t *testing.T) {
	ctx := context.Background()
	docUUID := "doc-123"
	ownerUUID := "owner-123"
	targetUUID := "target-456"

	tests := []struct {
		name        string
		setupMocks  func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository)
		expectError string
	}{
		{
			name: "Success",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(true, nil)
				userRepo.On("Exists", ctx, exec, targetUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, docUUID, ownerUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, docUUID).Return(nil)
			},
		},
		{
			name: "BeginTX error",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				docRepo.On("BeginTX", ctx).Return((*sqlx.Tx)(nil), func() error { return nil }, func() error { return nil }, errors.New("tx error"))
			},
			expectError: "не удалось начать транзакцию",
		},
		{
			name: "GetByUUID error",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return((*model.Document)(nil), []string(nil), errors.New("not found"))
			},
			expectError: "документ не найден",
		},
		{
			name: "Not owner",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				doc := &model.Document{UUID: docUUID, OwnerUUID: "other-owner"}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
			},
			expectError: "вы не являетесь владельцем документа",
		},
		{
			name: "Owner not exists",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(false, nil)
			},
			expectError: "владелец документа не найден",
		},
		{
			name: "Target user not exists",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(true, nil)
				userRepo.On("Exists", ctx, exec, targetUUID).Return(false, nil)
			},
			expectError: "пользователь для шаринга не найден",
		},
		{
			name: "AddGrant error",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(true, nil)
				userRepo.On("Exists", ctx, exec, targetUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, docUUID, ownerUUID, targetUUID).Return(errors.New("grant error"))
			},
			expectError: "ошибка изменения прав доступа",
		},
		{
			name: "Commit error",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return errors.New("commit error") }
				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(true, nil)
				userRepo.On("Exists", ctx, exec, targetUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, docUUID, ownerUUID, targetUUID).Return(nil)
			},
			expectError: "ошибка коммита транзакции",
		},
		{
			name: "Cache delete error",
			setupMocks: func(docRepo *MockDocumentRepository, userRepo *MockUserRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				doc := &model.Document{UUID: docUUID, OwnerUUID: ownerUUID}
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, docUUID, ownerUUID).Return(doc, []string{}, nil)
				userRepo.On("Exists", ctx, exec, ownerUUID).Return(true, nil)
				userRepo.On("Exists", ctx, exec, targetUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, docUUID, ownerUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, docUUID).Return(errors.New("cache error"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocRepo := new(MockDocumentRepository)
			mockUserRepo := new(MockUserRepository)
			mockGrantRepo := new(MockGrantRepository)
			mockCacheRepo := new(MockCacheRepository)

			tt.setupMocks(mockDocRepo, mockUserRepo, mockGrantRepo, mockCacheRepo)

			svc := service.NewDocumentService(mockDocRepo, mockCacheRepo, mockGrantRepo, nil, mockUserRepo, time.Minute)
			err := svc.ShareDocument(ctx, docUUID, ownerUUID, targetUUID)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockDocRepo.AssertExpectations(t)
			mockUserRepo.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
			mockCacheRepo.AssertExpectations(t)
		})
	}
}

func TestDeleteDocument_AllCases(t *testing.T) {
	ctx := context.Background()
	documentUUID := "doc-123"
	userUUID := "user-123"

	tests := []struct {
		name           string
		setupMocks     func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository)
		expectedResult map[string]bool
		expectError    string
	}{
		{
			name: "Success",
			setupMocks: func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, documentUUID, userUUID).Return(&model.Document{
					UUID:             documentUUID,
					OwnerUUID:        userUUID,
					FilenameOriginal: "file.txt",
					StoragePath:      "s3/file.txt",
				}, []string{}, nil)
				docRepo.On("Delete", ctx, exec, documentUUID, userUUID).Return(documentUUID, nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(nil)
				s3.On("DeleteObject", ctx, "s3/file.txt").Return(nil)
			},
			expectedResult: map[string]bool{documentUUID: true},
		},
		{
			name: "Not owner",
			setupMocks: func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, documentUUID, userUUID).Return(&model.Document{
					UUID:      documentUUID,
					OwnerUUID: "other-user",
				}, []string{}, nil)
			},
			expectError: "только владелец может удалить документ",
		},
		{
			name: "GetByUUID error",
			setupMocks: func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, documentUUID, userUUID).Return((*model.Document)(nil), []string{}, errors.New("db error"))
			},
			expectError: "документ не найден",
		},
		{
			name: "GetByUUID error",
			setupMocks: func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, documentUUID, userUUID).
					Return((*model.Document)(nil), []string{}, errors.New("db error"))
			},
			expectError: "документ не найден",
		},
		{
			name: "S3 delete error",
			setupMocks: func(docRepo *MockDocumentRepository, cacheRepo *MockCacheRepository, s3 *MockS3Storage, grantRepo *MockGrantRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				docRepo.On("GetByUUID", ctx, exec, documentUUID, userUUID).Return(&model.Document{
					UUID:             documentUUID,
					OwnerUUID:        userUUID,
					FilenameOriginal: "file.txt",
					StoragePath:      "s3/file.txt",
				}, []string{}, nil)
				docRepo.On("Delete", ctx, exec, documentUUID, userUUID).Return(documentUUID, nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(nil)
				s3.On("DeleteObject", ctx, "s3/file.txt").Return(errors.New("s3 error"))
			},
			expectError: "ошибка удаления файла из S3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			svc, docRepo, s3, cacheRepo, grantRepo := newTestDocumentServiceWithGrants()
			tt.setupMocks(docRepo, cacheRepo, s3, grantRepo)

			res, err := svc.DeleteDocument(ctx, documentUUID, userUUID)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, res)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expectedResult, res)
			}

			docRepo.AssertExpectations(t)
			cacheRepo.AssertExpectations(t)
			s3.AssertExpectations(t)
		})
	}
}

func TestListDocuments_AllCases(t *testing.T) {
	ctx := context.WithValue(context.Background(), "db", &config.Database{})
	userUUID := "user-123"
	login := "user_login"
	filterKey := ""
	filterValue := ""
	limit := 2

	tests := []struct {
		name           string
		setupMocks     func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, s3 *MockS3Storage)
		expectedDocs   []model.DocumentResponse
		expectedCursor string
		expectError    string
	}{
		{
			name: "Success with grants and presigned URL",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, s3 *MockS3Storage) {
				docs := []model.Document{
					{UUID: "doc1", FilenameOriginal: "file1.txt", StoragePath: "s3/file1.txt", IsFile: true, IsPublic: false, MimeType: "text/plain", CreatedAt: time.Now()},
					{UUID: "doc2", FilenameOriginal: "file2.txt", StoragePath: "s3/file2.txt", IsFile: true, IsPublic: true, MimeType: "text/plain", CreatedAt: time.Now()},
				}
				docRepo.On("ListDocuments", ctx, mock.Anything, userUUID, login, filterKey, filterValue, limit).Return(docs, nil)
				grantRepo.On("ListGrants", ctx, mock.Anything, "doc1").Return([]string{"userA"}, nil)
				grantRepo.On("ListGrants", ctx, mock.Anything, "doc2").Return([]string{"userB"}, nil)
				s3.On("GeneratePresignedGetURL", ctx, "s3/file1.txt", mock.Anything).Return("url1", nil)
				s3.On("GeneratePresignedGetURL", ctx, "s3/file2.txt", mock.Anything).Return("url2", nil)
			},
			expectedDocs: []model.DocumentResponse{
				{UUID: "doc1", Title: "file1.txt", PresignedURL: "url1", File: true, IsPublic: false, GrantLogins: []string{"userA"}, MimeType: "text/plain"},
				{UUID: "doc2", Title: "file2.txt", PresignedURL: "url2", File: true, IsPublic: true, GrantLogins: []string{"userB"}, MimeType: "text/plain"},
			},
			expectedCursor: "doc2",
		},
		{
			name: "DB error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, s3 *MockS3Storage) {
				docRepo.On("ListDocuments", ctx, mock.Anything, userUUID, login, filterKey, filterValue, limit).
					Return(nil, errors.New("db error"))
			},
			expectError: "не удалось получить список документов",
		},
		{
			name: "Grant error and S3 error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, s3 *MockS3Storage) {
				docs := []model.Document{
					{UUID: "doc1", FilenameOriginal: "file1.txt", StoragePath: "s3/file1.txt", IsFile: true, IsPublic: false, MimeType: "text/plain", CreatedAt: time.Now()},
				}
				docRepo.On("ListDocuments", ctx, mock.Anything, userUUID, login, filterKey, filterValue, limit).Return(docs, nil)
				grantRepo.On("ListGrants", ctx, mock.Anything, "doc1").Return([]string{}, errors.New("grant error"))
				s3.On("GeneratePresignedGetURL", ctx, "s3/file1.txt", mock.Anything).Return("", errors.New("s3 error"))
			},
			expectedDocs: []model.DocumentResponse{
				{UUID: "doc1", Title: "file1.txt", PresignedURL: "", File: true, IsPublic: false, GrantLogins: []string{}, MimeType: "text/plain"},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// создаём сервис с моками
			mockDocRepo := new(MockDocumentRepository)
			mockGrantRepo := new(MockGrantRepository)
			mockS3 := new(MockS3Storage)

			tt.setupMocks(mockDocRepo, mockGrantRepo, mockS3)

			svc := service.NewDocumentService(mockDocRepo, nil, mockGrantRepo, mockS3, nil, time.Minute)
			res, nextCursor, err := svc.ListDocuments(ctx, userUUID, login, filterKey, filterValue, limit)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
				assert.Nil(t, res)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, len(tt.expectedDocs), len(res))
				for i := range res {
					assert.Equal(t, tt.expectedDocs[i].UUID, res[i].UUID)
					assert.Equal(t, tt.expectedDocs[i].Title, res[i].Title)
					assert.Equal(t, tt.expectedDocs[i].PresignedURL, res[i].PresignedURL)
					assert.Equal(t, tt.expectedDocs[i].GrantLogins, res[i].GrantLogins)
				}
				assert.Equal(t, tt.expectedCursor, nextCursor)
			}

			mockDocRepo.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
			mockS3.AssertExpectations(t)
		})
	}
}

func TestAddGrant_AllCases(t *testing.T) {
	ctx := context.Background()
	documentUUID := "doc-123"
	ownerUUID := "owner-123"
	targetUUID := "target-456"

	tests := []struct {
		name        string
		setupMocks  func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository)
		expectError string
	}{
		{
			name: "Success",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, documentUUID, ownerUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(nil)
			},
		},
		{
			name: "BeginTX error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				docRepo.On("BeginTX", ctx).Return((*sqlx.Tx)(nil), func() error { return nil }, func() error { return nil }, errors.New("tx error"))
			},
			expectError: "ошибка начала транзакции",
		},
		{
			name: "Not owner",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(false, nil)
			},
			expectError: "доступ запрещён",
		},
		{
			name: "CheckOwner error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(false, errors.New("check error"))
			},
			expectError: "check error",
		},
		{
			name: "AddGrant error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, documentUUID, ownerUUID, targetUUID).Return(errors.New("add error"))
			},
			expectError: "не удалось добавить доступ к документу",
		},
		{
			name: "Commit error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return errors.New("commit error") }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, documentUUID, ownerUUID, targetUUID).Return(nil)
			},
			expectError: "ошибка коммита транзакции",
		},
		{
			name: "Cache delete error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("AddGrant", ctx, exec, documentUUID, ownerUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(errors.New("cache error"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocRepo := new(MockDocumentRepository)
			mockGrantRepo := new(MockGrantRepository)
			mockCache := new(MockCacheRepository)

			tt.setupMocks(mockDocRepo, mockGrantRepo, mockCache)

			svc := service.NewDocumentService(mockDocRepo, mockCache, mockGrantRepo, nil, nil, time.Minute)
			err := svc.AddGrant(ctx, documentUUID, ownerUUID, targetUUID)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockDocRepo.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
			mockCache.AssertExpectations(t)
		})
	}
}

func TestRemoveGrant_AllCases(t *testing.T) {
	ctx := context.Background()
	documentUUID := "doc-123"
	ownerUUID := "owner-123"
	targetUUID := "target-456"

	tests := []struct {
		name        string
		setupMocks  func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository)
		expectError string
	}{
		{
			name: "Success",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }

				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("RemoveGrant", ctx, exec, documentUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(nil)
			},
		},
		{
			name: "BeginTX error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				docRepo.On("BeginTX", ctx).Return((*sqlx.Tx)(nil), func() error { return nil }, func() error { return nil }, errors.New("tx error"))
			},
			expectError: "ошибка начала транзакции",
		},
		{
			name: "Not owner",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(false, nil)
			},
			expectError: "доступ запрещён",
		},
		{
			name: "CheckOwner error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(false, errors.New("check error"))
			},
			expectError: "check error",
		},
		{
			name: "RemoveGrant error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("RemoveGrant", ctx, exec, documentUUID, targetUUID).Return(errors.New("remove error"))
			},
			expectError: "remove error",
		},
		{
			name: "Commit error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return errors.New("commit error") }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("RemoveGrant", ctx, exec, documentUUID, targetUUID).Return(nil)
			},
			expectError: "ошибка коммита транзакции",
		},
		{
			name: "Cache delete error",
			setupMocks: func(docRepo *MockDocumentRepository, grantRepo *MockGrantRepository, cacheRepo *MockCacheRepository) {
				exec := new(sqlx.Tx)
				rollback := func() error { return nil }
				commit := func() error { return nil }
				docRepo.On("BeginTX", ctx).Return(exec, rollback, commit, nil)
				grantRepo.On("CheckOwner", ctx, exec, documentUUID, ownerUUID).Return(true, nil)
				grantRepo.On("RemoveGrant", ctx, exec, documentUUID, targetUUID).Return(nil)
				cacheRepo.On("DeleteDocument", ctx, documentUUID).Return(errors.New("cache error"))
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDocRepo := new(MockDocumentRepository)
			mockGrantRepo := new(MockGrantRepository)
			mockCache := new(MockCacheRepository)

			tt.setupMocks(mockDocRepo, mockGrantRepo, mockCache)

			svc := service.NewDocumentService(mockDocRepo, mockCache, mockGrantRepo, nil, nil, time.Minute)
			err := svc.RemoveGrant(ctx, documentUUID, ownerUUID, targetUUID)

			if tt.expectError != "" {
				assert.Error(t, err)
				assert.Contains(t, err.Error(), tt.expectError)
			} else {
				assert.NoError(t, err)
			}

			mockDocRepo.AssertExpectations(t)
			mockGrantRepo.AssertExpectations(t)
			mockCache.AssertExpectations(t)
		})
	}
}
