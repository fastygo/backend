package router

import (
	"github.com/fasthttp/router"
	"github.com/valyala/fasthttp"

	apiHandler "github.com/e-go/backend/api/handler"
)

type Handlers struct {
	Auth    *apiHandler.AuthHandler
	Profile *apiHandler.ProfileHandler
	Task    *apiHandler.TaskHandler
	Health  *apiHandler.HealthHandler
}

func New(handlers Handlers, authMiddleware func(fasthttp.RequestHandler) fasthttp.RequestHandler) *router.Router {
	r := router.New()

	r.GET("/health", handlers.Health.Check)

	// Auth routes
	r.POST("/api/v1/auth/login", handlers.Auth.Login)
	r.POST("/api/v1/auth/refresh", handlers.Auth.Refresh)

	// Protected routes
	r.GET("/api/v1/profile", authMiddleware(handlers.Profile.GetProfile))
	r.PUT("/api/v1/profile", authMiddleware(handlers.Profile.UpdateProfile))

	r.GET("/api/v1/tasks", authMiddleware(handlers.Task.GetTasks))
	r.POST("/api/v1/tasks", authMiddleware(handlers.Task.CreateTask))
	r.PUT("/api/v1/tasks/{id}", authMiddleware(handlers.Task.UpdateTask))
	r.DELETE("/api/v1/tasks/{id}", authMiddleware(handlers.Task.DeleteTask))

	return r
}

