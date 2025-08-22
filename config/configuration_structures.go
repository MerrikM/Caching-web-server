package config

import "github.com/aws/aws-sdk-go-v2/service/s3"

type ServerConfig struct {
	Host     string `yaml:"host"`
	Port     string `yaml:"port"`
	BasePath string `yaml:"base_path"`
}

type DatabaseConfig struct {
	DSN string `yaml:"dsn"`
}

type S3Config struct {
	Bucket   string `yaml:"bucket"`
	Client   *s3.Client
	Region   string `yaml:"region"`
	Endpoint string `yaml:"endpoint"`
	Local    bool   `yaml:"local"`
}

type JWTConfig struct {
	SecretKey       string `yaml:"secret_key"`
	AccessTokenTTL  string `yaml:"access_token_ttl"`
	RefreshTokenTTL string `yaml:"refresh_token_ttl"`
}

type WebhookConfig struct {
	URL     string `yaml:"url"`
	Timeout string `yaml:"timeout"`
}
