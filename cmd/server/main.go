package main

import (
	"context"
	"log"
	"time"

	"github.com/valyala/fasthttp"
	"go.uber.org/zap"

	apiHandler "github.com/fastygo/backend/api/handler"
	"github.com/fastygo/backend/internal/config"
	"github.com/fastygo/backend/internal/infrastructure/buffer"
	"github.com/fastygo/backend/internal/infrastructure/monitor"
	pgInfra "github.com/fastygo/backend/internal/infrastructure/postgres"
	redisInfra "github.com/fastygo/backend/internal/infrastructure/redis"
	"github.com/fastygo/backend/internal/middleware"
	"github.com/fastygo/backend/internal/router"
	"github.com/fastygo/backend/internal/services"
	"github.com/fastygo/backend/internal/services/lifecycle"
	"github.com/fastygo/backend/pkg/httpcontext"
	"github.com/fastygo/backend/pkg/logger"
	"github.com/fastygo/backend/repository/postgres"
	redisRepo "github.com/fastygo/backend/repository/redis"
	authUC "github.com/fastygo/backend/usecase/auth"
	profileUC "github.com/fastygo/backend/usecase/profile"
	taskUC "github.com/fastygo/backend/usecase/task"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	zapLogger, err := logger.New(logger.Config{
		Level:    cfg.Logger.Level,
		Encoding: cfg.Logger.Encoding,
	})
	if err != nil {
		log.Fatalf("logger error: %v", err)
	}
	defer zapLogger.Sync()

	appCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager := lifecycle.New(cfg.Context.ShutdownTimeout, zapLogger)
	manager.Listen(cancel)

	if err := pgInfra.RunMigrations(cfg, zapLogger); err != nil {
		zapLogger.Fatal("migrations failed", zap.Error(err))
	}

	pool, err := pgInfra.NewPool(appCtx, cfg.Database, zapLogger)
	if err != nil {
		zapLogger.Fatal("postgres connection failed", zap.Error(err))
	}
	manager.Register("postgres", func(ctx context.Context) error {
		pool.Close()
		return nil
	})

	redisClient, err := redisInfra.NewClient(cfg.Redis)
	if err != nil {
		zapLogger.Fatal("redis connection failed", zap.Error(err))
	}
	manager.Register("redis", func(ctx context.Context) error {
		return redisClient.Close()
	})

	bufferStore, err := buffer.Open(cfg.Buffer.Path, "buffer")
	if err != nil {
		zapLogger.Fatal("failed to open buffer store", zap.Error(err))
	}
	manager.Register("buffer", func(ctx context.Context) error {
		return bufferStore.Close()
	})

	mon := monitor.New(pool, redisClient, bufferStore, 10*time.Second, zapLogger)
	mon.Start()
	manager.Register("monitor", func(ctx context.Context) error {
		mon.Stop()
		return nil
	})

	userRepo := postgres.NewUserRepository(pool)
	taskRepo := postgres.NewTaskRepository(pool)
	sessionRepo := redisRepo.NewSessionRepository(redisClient, 24*time.Hour)

	bufferProcessor := services.NewBufferProcessor(
		bufferStore,
		mon,
		userRepo,
		taskRepo,
		zapLogger,
		services.ProcessorConfig{
			Interval:   cfg.Buffer.SyncInterval,
			BatchSize:  50,
			MaxRetries: cfg.Buffer.MaxRetry,
		},
	)
	bufferProcessor.Start()
	manager.Register("buffer_processor", func(ctx context.Context) error {
		bufferProcessor.Stop(ctx)
		return nil
	})

	bufferBridge := services.NewBufferBridge(bufferProcessor)

	authUseCase := authUC.New(userRepo, sessionRepo, zapLogger)
	profileUseCase := profileUC.New(userRepo, bufferBridge, zapLogger)
	taskUseCase := taskUC.New(taskRepo, bufferBridge, zapLogger)

	ctxAdapter := httpcontext.NewAdapter(cfg.Context.RequestTimeout)

	handlers := router.Handlers{
		Auth:    apiHandler.NewAuthHandler(authUseCase, ctxAdapter, zapLogger, time.Hour),
		Profile: apiHandler.NewProfileHandler(profileUseCase, ctxAdapter, zapLogger),
		Task:    apiHandler.NewTaskHandler(taskUseCase, ctxAdapter, zapLogger),
		Health:  apiHandler.NewHealthHandler(mon, ctxAdapter, zapLogger),
	}

	authMiddleware := middleware.JWTAuth(cfg.JWT.Secret, zapLogger)
	r := router.New(handlers, authMiddleware)

	server := &fasthttp.Server{
		Handler:      r.Handler,
		ReadTimeout:  cfg.HTTP.ReadTimeout,
		WriteTimeout: cfg.HTTP.WriteTimeout,
		IdleTimeout:  cfg.HTTP.IdleTimeout,
		Name:         cfg.AppName,
	}

	go func() {
		zapLogger.Info("server started", zap.String("address", cfg.Address()))
		if err := server.ListenAndServe(cfg.Address()); err != nil {
			zapLogger.Fatal("server crashed", zap.Error(err))
		}
	}()

	manager.Register("http_server", func(ctx context.Context) error {
		return server.Shutdown()
	})

	<-appCtx.Done()

	if err := manager.Shutdown(context.Background()); err != nil {
		zapLogger.Error("graceful shutdown error", zap.Error(err))
	}
}
