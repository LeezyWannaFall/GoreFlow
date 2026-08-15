package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/executor"
	"github.com/LeezyWannaFall/GoreFlow/internal/storage/postgres"
	"github.com/LeezyWannaFall/GoreFlow/internal/worker"
)

func main() {
	db, err := sql.Open("postgres", os.Getenv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("Failed to connect to the database: %v", err)
	}
	defer db.Close()

	if err := db.Ping(); err != nil {
		log.Fatalf("Failed to ping the database: %v", err)
	}

	log.Println("Successfully connected to the database.")

	repo := postgres.NewRepository(db)

	registry := executor.NewRegistry()
	err = registry.Register("echo", executor.NewEchoExecutor())
	if err != nil {
		log.Fatalf("Failed to register executor: %v", err)
	}

	processor := application.NewJobProcessor(repo, registry)

	workerID := uuid.New().String()
	pollInterval := 5 * time.Second
	leaseDuration := 30 * time.Second

	w := worker.NewWorker(workerID, processor, pollInterval, leaseDuration)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := w.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		log.Fatalf("Worker encountered an error: %v", err)
	} else {
		log.Println("Worker stopped gracefully.")
	}
}
