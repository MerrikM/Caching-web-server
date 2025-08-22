package config

import (
	"context"
	"fmt"
	"github.com/jmoiron/sqlx"
	_ "github.com/lib/pq"
	"log"
	"net/http"
)

type Database struct {
	*sqlx.DB
}

func NewDatabaseConnection(dbDriver string, dbConnectionStr string) (*Database, error) {
	database, err := sqlx.Connect(dbDriver, dbConnectionStr)
	if err != nil {
		return nil, fmt.Errorf("ошибка подключения к БД: %w", err)
	}

	if err := database.Ping(); err != nil {
		return nil, fmt.Errorf("ошибка пинга БД: %w", err)
	}

	log.Println("Подключение к БД успешно выполнено")
	return &Database{
		database,
	}, nil
}

func DBMiddleware(db *Database) func(next http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), "db", db)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func (db *Database) Close() error {
	err := db.DB.Close()
	if err != nil {
		return fmt.Errorf("ошибка закрытия соединения с БД: %w", err)
	}

	return nil
}
