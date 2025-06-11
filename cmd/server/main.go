package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/nazarioz/oidc-radius-bridge/config"
	"github.com/nazarioz/oidc-radius-bridge/internal/api"
	"github.com/nazarioz/oidc-radius-bridge/internal/auth"
	"github.com/nazarioz/oidc-radius-bridge/pkg/logger"
)

func main() {
	// Load environment variables
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Initialize logger
	logger := logger.NewLogger()

	// Load configuration
	cfg, err := config.LoadConfig()
	if err != nil {
		logger.Error("Failed to load configuration: %v", err)
		os.Exit(1)
	}

	// Initialize OIDC provider
	oidcProvider, err := auth.NewOIDCProvider(cfg, logger)
	if err != nil {
		logger.Error("Failed to initialize OIDC provider: %v", err)
		os.Exit(1)
	} else {
		logger.Info("OIDC provider initialized successfully")
	}

	// Initialize auth service
	authService := auth.NewOIDCService(oidcProvider)

	// Initialize HTTP handler
	handler := api.NewHandler(authService, logger)

	// Setup HTTP server for local FreeRADIUS communication
	server := &http.Server{
		Addr:         "127.0.0.1:8080", // Only listen on localhost
		Handler:      handler,
		ReadTimeout:  5 * time.Second,  // Shorter timeout for local requests
		WriteTimeout: 5 * time.Second,  // Shorter timeout for local requests
		IdleTimeout:  30 * time.Second, // Shorter idle timeout for local connections
	}

	// Channel to listen for errors coming from the listener
	serverErrors := make(chan error, 1)

	// Start the service listening for requests
	go func() {
		logger.Info("Starting OIDC-RADIUS bridge server on 127.0.0.1:8080...")
		serverErrors <- server.ListenAndServe()
	}()

	// Channel to listen for an interrupt or terminate signal from the OS
	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, os.Interrupt, syscall.SIGTERM)

	// Blocking main and waiting for shutdown
	select {
	case err := <-serverErrors:
		logger.Error("Error starting server: %v", err)

	case sig := <-shutdown:
		logger.Info("Shutdown signal received: %v", sig)
		// Give outstanding requests a deadline for completion
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		// Asking listener to shut down and shed load
		if err := server.Shutdown(ctx); err != nil {
			logger.Error("Graceful shutdown did not complete in 5s: %v", err)
			if err := server.Close(); err != nil {
				logger.Error("Could not stop server: %v", err)
			}
		}
	}
}
