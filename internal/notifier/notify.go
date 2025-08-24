package notifier

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type WebhookNotify struct {
	UserUUID  string
	NewIP     string
	OldIP     string
	Event     string
	TimeStamp string
}

func NotifyWebhook(webhookURL string, userUUID string, newIP string, oldIP string) error {
	payload := &WebhookNotify{
		UserUUID:  userUUID,
		NewIP:     newIP,
		OldIP:     oldIP,
		Event:     "refresh_token_from_new_ip",
		TimeStamp: time.Now().Format(time.RFC3339),
	}

	jsonBody, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("ошибка преобразования в json: %w", err)
	}

	response, err := http.Post(webhookURL, "application/json", bytes.NewBuffer(jsonBody))
	if err != nil {
		return fmt.Errorf("ошибка отправки webhook: %w", err)
	}
	defer response.Body.Close()

	log.Print("webhook успешно отправлен")
	return nil
}
