package config

import (
	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"
	"net/http"
	"os"
)

type AppConfig struct {
	DatabaseConfig DatabaseConfig `yaml:"databaseConfig"`
	RedisConfig    RedisConfig    `yaml:"redisConfig"`
	ServerAddr     string         `yaml:"serverAddr"`
	S3Config       S3Config       `yaml:"s3Config"`
	JWT            JWTConfig      `yaml:"jwt"`
	Webhook        WebhookConfig  `yaml:"webhook"`
	Admin          AdminConfig    `yaml:"admin"`
	TTL            TTL            `yaml:"TTL"`
}

func LoadConfig(path string) (*AppConfig, error) {
	file, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(file, &cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}

func SetupServer(serverAddress string) (*http.Server, *chi.Mux) {
	router := chi.NewRouter()
	server := &http.Server{
		Addr:    serverAddress,
		Handler: router,
	}

	return server, router
}

func SetupDatabase(dsn string) (*Database, error) {
	return NewDatabaseConnection("postgres", dsn)
}

func SetupRedis(cfg *RedisConfig) (*RedisClient, error) {
	return NewRedisClient(cfg)
}
