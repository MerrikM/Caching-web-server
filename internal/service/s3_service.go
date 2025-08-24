package service

import (
	"caching-web-server/config"
	"caching-web-server/internal/util"
	"context"
	"github.com/aws/aws-sdk-go-v2/aws"
	awsConfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"log"
	"time"
)

type S3Service struct {
	client   *s3.Client
	bucket   string
	psClient *s3.PresignClient
}

func NewS3Service(ctx context.Context, cfg *config.S3Config) (*S3Service, error) {
	var client *s3.Client

	if cfg.Local {
		client = s3.New(s3.Options{
			Region: cfg.Region,
			Credentials: credentials.NewStaticCredentialsProvider(
				"minioadmin",
				"minioadmin",
				"",
			),
			BaseEndpoint: aws.String(cfg.Endpoint),
			UsePathStyle: true,
		})

		if err := createBucketIfNotExists(ctx, client, cfg.Bucket); err != nil {
			return nil, util.LogError("[S3Service] ошибка создания бакета", err)
		}
	} else {
		awsCfg, err := awsConfig.LoadDefaultConfig(ctx, awsConfig.WithRegion(cfg.Region))
		if err != nil {
			return nil, util.LogError("[S3Service]  загрузки AWS config: %w", err)
		}
		client = s3.NewFromConfig(awsCfg)
	}

	psClient := s3.NewPresignClient(client)

	return &S3Service{
		client:   client,
		psClient: psClient,
		bucket:   cfg.Bucket,
	}, nil
}

// createBucketIfNotExists создает бакет если он не существует
func createBucketIfNotExists(ctx context.Context, client *s3.Client, bucket string) error {
	_, err := client.HeadBucket(ctx, &s3.HeadBucketInput{
		Bucket: aws.String(bucket),
	})

	if err == nil {
		return nil // Бакет уже существует
	}

	_, err = client.CreateBucket(ctx, &s3.CreateBucketInput{
		Bucket: aws.String(bucket),
	})

	if err != nil {
		return util.LogError("[S3Service] ошибка создания бакета", err)
	}

	log.Printf("[S3Service] бакет %s успешно создан", bucket)
	return nil
}

// GeneratePresignedGetURL : генерация pre-signed URL для GET
func (s *S3Service) GeneratePresignedGetURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	req, err := s.psClient.PresignGetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expire
	})
	if err != nil {
		return "", util.LogError("[S3Service] не удалось сгенерировать presigned GET URL", err)
	}

	return req.URL, nil
}

// GeneratePresignedPutURL : генерация pre-signed URL для PUT
func (s *S3Service) GeneratePresignedPutURL(ctx context.Context, key string, expire time.Duration) (string, error) {
	req, err := s.psClient.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	}, func(opts *s3.PresignOptions) {
		opts.Expires = expire
	})
	if err != nil {
		return "", util.LogError("[S3Service] не удалось сгенерировать presigned PUT URL", err)
	}
	return req.URL, nil
}

// DeleteObject : удаление объекта
func (s *S3Service) DeleteObject(ctx context.Context, key string) error {
	_, err := s.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(s.bucket),
		Key:    aws.String(key),
	})
	if err != nil {
		return util.LogError("[S3Service] не удалось удалить объект", err)
	}
	return nil
}
