package util

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type S3Uploader struct {
	client   *http.Client
	wg       sync.WaitGroup
	errors   chan error
	progress chan int64
}

func NewS3Uploader() *S3Uploader {
	return &S3Uploader{
		client: &http.Client{
			Timeout: 60 * time.Minute, // Для очень больших файлов
		},
		errors:   make(chan error, 10),  // Буферизированный канал для ошибок
		progress: make(chan int64, 100), // Канал для прогресса
	}
}

// UploadFileAsync асинхронная загрузка файла
func (u *S3Uploader) UploadFileAsync(presignedURL string, filePath string) {
	u.wg.Add(1)

	go func() {
		defer u.wg.Done()

		err := u.uploadFile(presignedURL, filePath)
		if err != nil {
			u.errors <- fmt.Errorf("ошибка загрузки %s: %w", filepath.Base(filePath), err)
		} else {
			u.progress <- -1 // Сигнал завершения
		}
	}()
}

// uploadFile синхронная реализация загрузки
func (u *S3Uploader) uploadFile(presignedURL string, filePath string) error {
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("ошибка открытия файла: %w", err)
	}
	defer file.Close()
	defer os.Remove(filePath)

	fileInfo, err := file.Stat()
	if err != nil {
		return fmt.Errorf("ошибка получения информации о файле: %w", err)
	}

	req, err := http.NewRequest("PUT", presignedURL, file)
	if err != nil {
		return fmt.Errorf("ошибка создания запроса: %w", err)
	}

	req.ContentLength = fileInfo.Size()
	req.Header.Set("Content-Type", getContentType(filePath))

	resp, err := u.client.Do(req)
	if err != nil {
		return fmt.Errorf("ошибка выполнения запроса: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ошибка загрузки: статус %d, ответ: %s", resp.StatusCode, string(body))
	}

	return nil
}

// getContentType определяет MIME type файла
func getContentType(filename string) string {
	ext := filepath.Ext(filename)
	switch ext {
	case ".txt":
		return "text/plain"
	case ".pdf":
		return "application/pdf"
	case ".jpg", ".jpeg":
		return "image/jpeg"
	case ".png":
		return "image/png"
	case ".zip":
		return "application/zip"
	case ".doc":
		return "application/msword"
	case ".docx":
		return "application/vnd.openxmlformats-officedocument.wordprocessingml.document"
	default:
		return "application/octet-stream"
	}
}

// Wait ожидание завершения всех загрузок
func (u *S3Uploader) Wait() error {
	u.wg.Wait()
	close(u.errors)
	close(u.progress)

	// Проверяем ошибки
	if len(u.errors) > 0 {
		return <-u.errors // Возвращаем первую ошибку
	}
	return nil
}

// Errors возвращает канал с ошибками
func (u *S3Uploader) Errors() <-chan error {
	return u.errors
}

// Progress возвращает канал с прогрессом
func (u *S3Uploader) Progress() <-chan int64 {
	return u.progress
}
