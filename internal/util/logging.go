package util

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
)

func LogError(message string, err error) error {
	log.Printf("%s: %v", message, err)
	return fmt.Errorf("%s: %w", message, err)
}

func HandleError(w http.ResponseWriter, message string, statusCode int) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)

	errorResponse := struct {
		Error   string `json:"error"`
		Message string `json:"message"`
		Code    int    `json:"code"`
	}{
		Error:   http.StatusText(statusCode),
		Message: message,
		Code:    statusCode,
	}

	json.NewEncoder(w).Encode(errorResponse)
}
