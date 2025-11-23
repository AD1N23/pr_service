package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/go-chi/chi/middleware"
	"github.com/go-chi/chi/v5"
	"main.go/internal/config"
	"main.go/internal/http-server/handlers/pr/merge"
	"main.go/internal/http-server/handlers/pr/reassign"
	PrSave "main.go/internal/http-server/handlers/pr/save"
	teamGet "main.go/internal/http-server/handlers/team/get"
	teamSave "main.go/internal/http-server/handlers/team/save"
	setactive "main.go/internal/http-server/handlers/users/set_active"
	getreview "main.go/internal/http-server/handlers/users/set_active/get"
	"main.go/internal/storage/postgres"
)

const (
	envLocal = "local"
	envProd  = "prod"
)

func main() {
	cfg := config.NewConfig()
	log := setupLogger(cfg.Env)
	log.Info("starting rv-service", slog.String("env", cfg.Env))

	storage, err := postgres.New(cfg.StoragePath)
	if err != nil {
		log.Error("failed to init storage")
		os.Exit(1)
	}

	router := chi.NewRouter()
	router.Use(middleware.RequestID)
	router.Use(middleware.Recoverer)
	router.Use(middleware.Logger)

	router.Post("/team/add", teamSave.New(log, storage))
	router.Get("/team/get", teamGet.New(log, storage))
	router.Post("/users/setIsActive", setactive.New(log, storage))
	router.Post("/pullRequest/create", PrSave.New(log, storage))
	router.Post("/pullRequest/merge", merge.New(log, storage))
	router.Post("/pullRequest/reassign", reassign.New(log, storage))
	router.Get("/users/getReview", getreview.New(log, storage))

	log.Info("starting server", slog.String("address", cfg.Address))

	srv := &http.Server{
		Addr:         cfg.Address,
		Handler:      router,
		ReadTimeout:  cfg.HTTPServer.Timeout,
		WriteTimeout: cfg.HTTPServer.Timeout,
		IdleTimeout:  cfg.HTTPServer.IdleTimeout,
	}

	if err := srv.ListenAndServe(); err != nil {
		log.Error("failed to start server")
	}

}

func setupLogger(env string) *slog.Logger {
	var log *slog.Logger
	switch env {
	case envLocal:
		log = slog.New(
			slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))
	case envProd:
		log = slog.New(
			slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	}
	return log
}
