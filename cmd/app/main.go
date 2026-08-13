package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/LeezyWannaFall/GoreFlow/internal/application"
	"github.com/LeezyWannaFall/GoreFlow/internal/storage/postgres"
	"github.com/LeezyWannaFall/GoreFlow/internal/transport/http"
	"github.com/go-chi/chi/v5"
	_ "github.com/lib/pq"
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
	app := application.NewApplication(repo)
	handler := httptransport.NewHandler(app)

	r := chi.NewRouter()
	r.Post("/jobs", handler.CreateJob)
	r.Get("/jobs/{id}", handler.GetJobByID)
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	server := &http.Server{
		Addr:              ":8080",
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	serverErrors := make(chan error, 1)
	go func() {
		serverErrors <- server.ListenAndServe()
	}()

	log.Printf("Starting server on %s", server.Addr)

	select {
	case err := <-serverErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
		return
	case <-shutdownSignal.Done():
		log.Println("Shutdown signal received")
	}

	stop()

	shutdownContext, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()

	if err := server.Shutdown(shutdownContext); err != nil {
		log.Printf("Graceful shutdown failed: %v", err)
		if closeErr := server.Close(); closeErr != nil {
			log.Printf("Force close failed: %v", closeErr)
		}
	}

	if err := <-serverErrors; err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("HTTP server stopped with error: %v", err)
	}

	log.Println("Server stopped")
}
