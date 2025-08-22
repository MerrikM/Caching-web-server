package repository

import (
	"caching-web-server/config"
	"caching-web-server/internal/model"
	"caching-web-server/internal/util"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/redis/go-redis/v9"
	"time"
)

type CacheRepository struct {
	client *config.RedisClient
	ttl    time.Duration
}

func NewCacheRepository(rdb *config.RedisClient, ttl time.Duration) *CacheRepository {
	return &CacheRepository{rdb, ttl}
}

func (r *CacheRepository) SetDocument(ctx context.Context, document *model.Document) error {
	data, err := json.Marshal(document)
	if err != nil {
		return util.LogError("ошибка сериализации заказа", err)
	}

	cmd := r.client.Client.Set(ctx, r.key(document.UUID), data, r.ttl)
	if err = cmd.Err(); err != nil {
		return util.LogError("ошибка сохранения в Redis", err)
	}
	if cmd.Val() != "OK" {
		return fmt.Errorf("неожиданный ответ Redis: %s", cmd.Val())
	}

	return nil
}

func (r *CacheRepository) GetDocument(ctx context.Context, uuid string) (*model.Document, error) {
	val, err := r.client.Client.Get(ctx, r.key(uuid)).Result()
	if errors.Is(err, redis.Nil) {
		return nil, nil // нет в кэше
	} else if err != nil {
		return nil, util.LogError("ошибка получения документа из Redis", err)
	}

	var order model.Document
	if err := json.Unmarshal([]byte(val), &order); err != nil {
		return nil, util.LogError("ошибка десериализации документа из кэша", err)
	}
	return &order, nil
}

func (r *CacheRepository) DeleteDocument(ctx context.Context, uuid string) error {
	if err := r.client.Client.Del(ctx, r.key(uuid)).Err(); err != nil {
		return util.LogError("ошибка удаления документа из Redis", err)
	}
	return nil
}

func (r *CacheRepository) Pipeline() redis.Pipeliner {
	return r.client.Client.Pipeline()
}

func (r *CacheRepository) key(uuid string) string {
	return fmt.Sprintf("document:%s", uuid)
}
