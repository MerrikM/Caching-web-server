package main

import (
	"caching-web-server/config"
	_ "caching-web-server/docs"
	"caching-web-server/internal/handler"
	"caching-web-server/internal/repository"
	"caching-web-server/internal/security"
	"caching-web-server/internal/service"
	"context"
	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
	httpSwagger "github.com/swaggo/http-swagger"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// @title Caching-web-server
// @version 1.0
// @description REST API Для работы с документами

// @host localhost:8080

// @securityDefinitions.apikey ApiKeyAuth
// @in header
// @name Authorization
func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := config.LoadConfig("config.yaml")
	if err != nil {
		log.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	db, err := config.SetupDatabase(cfg.DatabaseConfig.DSN)
	if err != nil {
		log.Fatalf("Не удалось подключиться к БД: %v", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Ошибка при закрытии БД: %v", err)
		}
	}()

	redisClient, err := config.SetupRedis(&cfg.RedisConfig)
	if err != nil {
		log.Fatalf("Ошибка подключения к Redis: %v", err)
	}
	defer func() {
		if err := redisClient.Close(); err != nil {
			log.Printf("Ошибка при закрытии Redis: %v", err)
		}
	}()

	srv, router := config.SetupServer(cfg.ServerAddr)

	userRepo := repository.NewUserRepository(db)
	jwtRepo := repository.NewJWTRepository(db)
	docRepo := repository.NewDocumentRepository(db)
	shareRepo := repository.NewGrantDocumentRepository(db)
	cacheRepo := repository.NewCacheRepository(redisClient, time.Duration(cfg.TTL.S3AndRedis)*time.Second)

	s3Service, err := service.NewS3Service(ctx, &cfg.S3Config)
	if err != nil {
		log.Fatalf("Ошибка создания S3 сервиса: %v", err)
	}
	docService := service.NewDocumentService(docRepo, cacheRepo, shareRepo, s3Service, userRepo, time.Duration(cfg.TTL.S3AndRedis)*time.Second)

	jwtService := security.NewJWTService(&cfg.JWT)
	userService := service.NewUserService(userRepo, jwtService, jwtRepo, &cfg.Admin)
	authService := service.NewAuthenticationService(jwtRepo, cfg, jwtService, userRepo)

	authHandler := handler.NewAuthenticationHandler(authService, jwtService, jwtRepo)
	docHandler := handler.NewDocumentHandler(docService, &cfg.TTL)
	userHandler := handler.NewUserHandler(userService)

	router.Use(config.DBMiddleware(db))
	router.Get("/swagger/*", httpSwagger.WrapHandler)

	setupAuthRoutes(router, authHandler, jwtService, jwtRepo, cfg)
	setupUserRoutes(router, userHandler, jwtService, jwtRepo, cfg)
	setupDocumentRoutes(router, docHandler, jwtService, jwtRepo, cfg)

	runServer(ctx, srv)
}

func setupAuthRoutes(r chi.Router, h *handler.AuthenticationHandler, jwtService *security.JWTService, jwtRepo *repository.JWTRepository, cfg *config.AppConfig) {
	r.Route("/api/auth", func(r chi.Router) {
		r.Group(func(r chi.Router) {
			r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepo, jwtService, cfg.Admin.AdminToken))
			r.Get("/me", h.GetCurrentUsersUUID)
			r.Head("/me", h.GetCurrentUsersUUIDHead)
			r.Post("/refresh", h.RefreshToken)
		})
		r.Group(func(r chi.Router) {
			r.Post("/", h.Login)
			r.Delete("/{token}", h.Logout)
		})
	})
}

func setupUserRoutes(r chi.Router, h *handler.UserHandler, jwtService *security.JWTService, jwtRepo *repository.JWTRepository, cfg *config.AppConfig) {
	r.Route("/api", func(r chi.Router) {
		r.Post("/register", h.RegisterUser)

		r.Group(func(r chi.Router) {
			r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepo, jwtService, cfg.Admin.AdminToken))

			r.Get("/users", h.ListUsers)
			r.Head("/users", h.ListUsers)

			r.Route("/users/{uuid}", func(r chi.Router) {
				r.Get("/", h.GetUser)
				r.Head("/", h.GetUserHead)
				r.Put("/", h.UpdateUser)
				r.Put("/password", h.UpdatePassword)
			})

			r.Delete("/users/{uuid}", h.DeleteUser)
		})
	})
}

func setupDocumentRoutes(r chi.Router, h *handler.DocumentHandler, jwtService *security.JWTService, jwtRepo *repository.JWTRepository, cfg *config.AppConfig) {
	r.Route("/api/docs", func(r chi.Router) {
		r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepo, jwtService, cfg.Admin.AdminToken))
		r.Get("/", h.ListDocuments)
		r.Head("/", h.ListDocumentsHead)
		r.Post("/", h.CreateDocument)

		r.Route("/{doc_id}", func(r chi.Router) {
			r.Get("/", h.GetDocument)
			r.Head("/", h.GetDocumentHead)
			r.Post("/share", h.ShareDocument)
			r.Post("/remove-grant", h.RemoveGrantFromDocument)
			r.Delete("/", h.DeleteDocument)
		})
	})

	r.Route("/public/docs", func(r chi.Router) {
		r.Get("/{doc_id}", h.GetPublicDocumentByUUID)
		r.Head("/{doc_id}", h.GetPublicDocumentByUUIDHead)
		r.Get("/token/{token}", h.GetPublicDocumentByToken)
		r.Head("/token/{token}", h.GetPublicDocumentByTokenHead)
	})

	r.Get("/api/docs/public/{token}", h.GetDocumentByToken)
}

//func main() {
//	ctx, cancel := context.WithCancel(context.Background())
//	defer cancel()
//
//	cfg, err := config.LoadConfig("config.yaml")
//	if err != nil {
//		log.Fatalf("ошибка загрузки конфигурации: %v", err)
//	}
//
//	database, err := config.SetupDatabase(cfg.DatabaseConfig.DSN)
//	if err != nil {
//		log.Fatalf("не удалось подключиться к БД: %v", err)
//	}
//	defer func() {
//		if err := database.Close(); err != nil {
//			log.Printf("ошибка при закрытии БД: %v", err)
//		}
//	}()
//
//	srv, router := config.SetupServer(cfg.ServerAddr)
//
//	redisClient, err := config.SetupRedis(&cfg.RedisConfig)
//	if err != nil {
//		log.Fatalf("ошибка подключения к Redis: %v", err)
//	}
//	defer func() {
//		if err := redisClient.Close(); err != nil {
//			log.Printf("ошибка при закрытии Redis: %v", err)
//		}
//	}()
//
//	docRepository := repository.NewDocumentRepository(database)
//	shareRepository := repository.NewGrantDocumentRepository(database)
//	userRepository := repository.NewUserRepository(database)
//	jwtRepository := repository.NewJWTRepository(database)
//	cacheRepository := repository.NewCacheRepository(redisClient, time.Duration(cfg.TTL.S3AndRedis)*time.Second)
//
//	s3Service, err := service.NewS3Service(context.Background(), &cfg.S3Config)
//	if err != nil {
//		log.Fatalf("Failed to create S3 service: %v", err)
//	}
//
//	docService := service.NewDocumentService(docRepository, cacheRepository, shareRepository, s3Service, userRepository, time.Duration(cfg.TTL.S3AndRedis)*time.Second)
//	_ = s3Service
//	_ = docService
//	_ = shareRepository
//	// JWT сервис и пользовательский сервис
//	jwtService := security.NewJWTService(&cfg.JWT) // пример, создаем реализацию JWTServiceInterface
//	userService := service.NewUserService(userRepository, jwtService, jwtRepository, &cfg.Admin)
//
//	// Authentication service
//	authService := service.NewAuthenticationService(jwtRepository, cfg, jwtService, userRepository)
//
//	authenticationHandler := handler.NewAuthenticationHandler(authService, jwtService, jwtRepository)
//	documentHandler := handler.NewDocumentHandler(docService, &cfg.TTL)
//
//	userHandler := handler.NewUserHandler(userService)
//	router.Use(config.DBMiddleware(database))
//
//	router.Get("/swagger/*", httpSwagger.WrapHandler)
//
//	router.Route("/api/auth", func(r chi.Router) {
//		r.Group(func(r chi.Router) {
//			r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepository, jwtService, cfg.Admin.AdminToken))
//			r.Get("/me", authenticationHandler.GetCurrentUsersUUID)
//			r.Head("/me", authenticationHandler.GetCurrentUsersUUIDHead)
//			r.Post("/refresh", authenticationHandler.RefreshToken)
//			r.Delete("/{token}", authenticationHandler.Logout)
//		})
//		r.Group(func(r chi.Router) {
//			r.Post("/", authenticationHandler.Login)
//		})
//	})
//
//	router.Route("/api/users", func(r chi.Router) {
//		r.Group(func(r chi.Router) {
//			r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepository, jwtService, cfg.Admin.AdminToken))
//			r.Get("/{uuid}", userHandler.GetUser)
//			r.Head("/{uuid}", userHandler.GetUserHead)
//			r.Put("/{uuid}", userHandler.UpdateUser)
//			r.Put("/{uuid}/password", userHandler.UpdatePassword)
//			r.Delete("/delete", userHandler.DeleteUser)
//		})
//	})
//
//	router.Route("/api/docs", func(r chi.Router) {
//		r.Use(security.JWTMiddleware([]byte(cfg.JWT.SecretKey), jwtRepository, jwtService, cfg.Admin.AdminToken))
//		r.Get("/", documentHandler.ListDocuments)
//		r.Head("/", documentHandler.ListDocumentsHead)
//		r.Post("/", documentHandler.CreateDocument)
//
//		r.Route("/{doc_id}", func(r chi.Router) {
//			r.Get("/", documentHandler.GetDocument)
//			r.Head("/", documentHandler.GetDocumentHead)
//			r.Post("/share", documentHandler.ShareDocument)
//			r.Post("/remove", documentHandler.RemoveGrantFromDocument)
//			r.Delete("/", documentHandler.DeleteDocument)
//		})
//	})
//
//	router.Route("/public/docs", func(r chi.Router) {
//		r.Get("/{doc_id}", documentHandler.GetPublicDocumentByUUID)
//		r.Head("/{doc_id}", documentHandler.GetPublicDocumentByUUIDHead)
//		r.Get("/token/{token}", documentHandler.GetPublicDocumentByToken)
//		r.Head("/token/{token}", documentHandler.GetPublicDocumentByTokenHead)
//	})
//
//	router.Get("/api/docs/public/{token}", documentHandler.GetDocumentByToken)
//
//	router.Route("/api/", func(r chi.Router) {
//		r.Post("/register", userHandler.RegisterUser)
//		r.Get("/users", userHandler.ListUsers)
//		r.Head("/users", userHandler.ListUsersHead)
//	})
//
//	runServer(ctx, srv)
//}

func runServer(ctx context.Context, server *http.Server) {
	serverErrors := make(chan error, 1)
	go func() {
		log.Println("сервер запущен на " + server.Addr)
		serverErrors <- server.ListenAndServe()
	}()

	signalChannel := make(chan os.Signal, 1)
	signal.Notify(signalChannel, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-serverErrors:
		if err != nil {
			log.Fatalf("ошибка работы сервера: %v", err)
		}
	case sig := <-signalChannel:
		log.Printf("получен сигнал %v остановки работы сервера ", sig)
	}

	shutDownCtx, shutDownCancel := context.WithTimeout(ctx, 5*time.Second)
	defer shutDownCancel()

	if err := server.Shutdown(shutDownCtx); err != nil {
		log.Printf("ошибка при остановке сервера: %v", err)
	} else {
		log.Println("Сервер успешно остановлен")
	}
}
