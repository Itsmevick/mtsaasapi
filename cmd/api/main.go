package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "github.com/lib/pq"
	"github.com/multitenant-saas/api/internal/config"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/handlers"
	"github.com/multitenant-saas/api/internal/logger"
	"github.com/multitenant-saas/api/internal/middleware"
	"github.com/multitenant-saas/api/internal/router"
	"github.com/multitenant-saas/api/internal/services"
	"github.com/rs/zerolog/log"
)

func main() {
	// Load configuration
	cfg := config.Load()

	// Initialize logger
	logger.Init(cfg.Env)

	// Initialize database
	db, err := database.New(cfg.DatabaseURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to database")
	}
	defer db.Close()

	// Run migrations
	if err := database.RunMigrations(cfg.DatabaseURL); err != nil {
		log.Fatal().Err(err).Msg("Failed to run migrations")
	}

	// Initialize Redis
	redisClient, err := database.NewRedis(cfg.RedisURL)
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect to Redis")
	}
	defer redisClient.Close()

	// Initialize services
	authService := services.NewAuthService(db, redisClient, cfg)
	workspaceService := services.NewWorkspaceService(db)
	memberService := services.NewMemberService(db)
	projectService := services.NewProjectService(db)
	taskService := services.NewTaskService(db)
	activityService := services.NewActivityService(db)
	billingService := services.NewBillingService(db, cfg)
	dashboardService := services.NewDashboardService(db)

	// Initialize handlers
	h := handlers.New(
		authService,
		workspaceService,
		memberService,
		projectService,
		taskService,
		activityService,
		billingService,
		dashboardService,
		cfg,
	)

	// Initialize middleware
	mw := middleware.New(authService, cfg, redisClient)

	// Setup router
	r := router.Setup(h, mw, cfg)

	// Start server
	srv := &http.Server{
		Addr:         fmt.Sprintf(":%d", cfg.Port),
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in goroutine
	go func() {
		log.Info().Int("port", cfg.Port).Msg("Server starting")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal().Err(err).Msg("Server failed to start")
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Info().Msg("Server shutting down")

	// Graceful shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		log.Error().Err(err).Msg("Server forced to shutdown")
	}

	log.Info().Msg("Server exited")
}
