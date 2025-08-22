package util

import (
	"fmt"
	"log"
)

func LogError(message string, err error) error {
	log.Printf("%s: %v", message, err)
	return fmt.Errorf("%s: %w", message, err)
}
