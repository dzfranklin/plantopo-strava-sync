package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/handlers"
	"plantopo-strava-sync/internal/metrics"
	"plantopo-strava-sync/internal/middleware"
	"plantopo-strava-sync/internal/oauth"
	"plantopo-strava-sync/internal/strava"
	"plantopo-strava-sync/internal/worker"
)

func main() {
	// Define CLI flags
	listSubscriptions := flag.Bool("list-strava-subscriptions", false, "List all Strava webhook subscriptions")
	deleteSubscription := flag.String("delete-strava-subscription", "", "Delete a Strava webhook subscription by ID")
	createSubscription := flag.String("create-strava-subscription", "", "Create a Strava webhook subscription with callback URL")

	flag.Parse()

	// Check if any CLI command was requested
	if *listSubscriptions || *deleteSubscription != "" || *createSubscription != "" {
		runCLI(*listSubscriptions, *deleteSubscription, *createSubscription)
		return
	}

	// Otherwise, start the server
	runServer()
}

func runCLI(listSubs bool, deleteSub, createSub string) {
	// Disable structured logging for CLI
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors
	})))

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to load configuration: %v\n", err)
		os.Exit(1)
	}

	// Open database (needed for client initialization)
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to open database: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	// Create Strava client
	client := strava.NewClient(cfg, db)

	// Handle commands
	switch {
	case listSubs:
		handleListSubscriptions(client)
	case deleteSub != "":
		handleDeleteSubscription(client, deleteSub)
	case createSub != "":
		handleCreateSubscription(client, cfg, createSub)
	}
}

func handleListSubscriptions(client *strava.Client) {
	fmt.Println("Fetching subscriptions...")

	subscriptions, err := client.ListSubscriptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list subscriptions: %v\n", err)
		os.Exit(1)
	}

	if len(subscriptions) == 0 {
		fmt.Println("No active subscriptions found.")
		return
	}

	fmt.Printf("\nFound %d subscription(s):\n\n", len(subscriptions))
	for _, sub := range subscriptions {
		fmt.Printf("ID: %d\n", sub.ID)
		fmt.Printf("  Application ID: %d\n", sub.ApplicationID)
		fmt.Printf("  Callback URL: %s\n", sub.CallbackURL)
		fmt.Printf("  Created: %s\n", sub.CreatedAt)
		fmt.Printf("  Updated: %s\n", sub.UpdatedAt)
		fmt.Println()
	}
}

func handleDeleteSubscription(client *strava.Client, idStr string) {
	subscriptionID, err := strconv.Atoi(idStr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid subscription ID: %s\n", idStr)
		os.Exit(1)
	}

	fmt.Printf("Deleting subscription %d...\n", subscriptionID)

	err = client.DeleteSubscription(subscriptionID)
	if err != nil {
		if httpErr, ok := err.(*strava.HTTPError); ok && httpErr.StatusCode == 404 {
			fmt.Fprintf(os.Stderr, "Error: Subscription %d not found\n", subscriptionID)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("✓ Subscription deleted successfully!")
}

func handleCreateSubscription(client *strava.Client, cfg *config.Config, callbackURL string) {
	fmt.Printf("Creating webhook subscription...\n")
	fmt.Printf("Callback URL: %s\n", callbackURL)
	fmt.Printf("Verify Token: %s\n", cfg.StravaVerifyToken)
	fmt.Println()

	subscription, err := client.CreateSubscription(callbackURL, cfg.StravaVerifyToken)
	if err != nil {
		if httpErr, ok := err.(*strava.HTTPError); ok {
			fmt.Fprintf(os.Stderr, "Error: Subscription creation failed (HTTP %d)\n", httpErr.StatusCode)
			fmt.Fprintf(os.Stderr, "Response: %s\n", httpErr.Body)

			if httpErr.StatusCode == 400 {
				fmt.Fprintln(os.Stderr, "\nPossible issues:")
				fmt.Fprintln(os.Stderr, "- A subscription already exists for this application")
				fmt.Fprintln(os.Stderr, "- The callback URL is not accessible from Strava")
				fmt.Fprintln(os.Stderr, "- The verify token does not match")
			}
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("✓ Subscription created successfully!")
	fmt.Printf("  ID: %d\n", subscription.ID)
	fmt.Printf("  Application ID: %d\n", subscription.ApplicationID)
	fmt.Printf("  Callback URL: %s\n", subscription.CallbackURL)
	fmt.Printf("  Created At: %s\n", subscription.CreatedAt)
}

func runServer() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		slog.Error("Failed to load configuration", "error", err)
		os.Exit(1)
	}

	// Set up logger
	logLevel := slog.LevelInfo
	switch cfg.LogLevel {
	case "debug":
		logLevel = slog.LevelDebug
	case "warn":
		logLevel = slog.LevelWarn
	case "error":
		logLevel = slog.LevelError
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	}))
	slog.SetDefault(logger)

	logger.Info("Starting plantopo-strava-sync server",
		"host", cfg.Host,
		"port", cfg.Port,
		"database", cfg.DatabasePath,
		"log_level", cfg.LogLevel)

	// Open database
	db, err := database.Open(cfg.DatabasePath)
	if err != nil {
		logger.Error("Failed to open database", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	logger.Info("Database opened successfully")

	// Create Strava client
	stravaClient := strava.NewClient(cfg, db)

	// Create OAuth manager
	oauthManager := oauth.NewManager(cfg, db, stravaClient)

	// Create handlers
	oauthHandler := handlers.NewOAuthHandler(oauthManager, cfg)
	webhookHandler := handlers.NewWebhookHandler(db, cfg)
	eventsHandler := handlers.NewEventsHandler(db, cfg)

	// Set up HTTP routes
	mux := http.NewServeMux()

	// OAuth endpoints
	mux.Handle("/oauth-start", middleware.WrapHandler(metrics.EndpointOAuthStart, oauthHandler.HandleAuthStart))
	mux.Handle("/oauth-callback", middleware.WrapHandler(metrics.EndpointOAuthCallback, oauthHandler.HandleCallback))

	// Webhook endpoints
	mux.Handle("/webhook-callback", middleware.WrapHandler(metrics.EndpointWebhook, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			// GET = verification
			webhookHandler.HandleVerification(w, r)
		} else if r.Method == http.MethodPost {
			// POST = event
			webhookHandler.HandleEvent(w, r)
		} else {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		}
	}))

	// Events API endpoint
	mux.Handle("/events", middleware.WrapHandler(metrics.EndpointEvents, eventsHandler.HandleEvents))

	// Health check endpoint
	mux.Handle("/health", middleware.WrapHandler(metrics.EndpointHealth, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	}))

	// Create HTTP server
	addr := fmt.Sprintf("%s:%d", cfg.Host, cfg.Port)
	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  35 * time.Second, // Slightly more than long-poll timeout
		WriteTimeout: 35 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	// Start webhook worker in background
	workerInstance := worker.NewWorker(db, stravaClient)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()

	go func() {
		logger.Info("Starting webhook worker")
		if err := workerInstance.Start(workerCtx); err != nil && err != context.Canceled {
			logger.Error("Webhook worker failed", "error", err)
		}
	}()

	// Start queue depth collector if metrics are enabled
	if cfg.MetricsEnabled {
		go func() {
			logger.Info("Starting queue depth collector")
			metrics.StartQueueDepthCollector(workerCtx, db, 15*time.Second)
		}()
	}

	// Start metrics server if enabled
	var metricsServer *http.Server
	if cfg.MetricsEnabled {
		metricsMux := http.NewServeMux()
		metricsMux.Handle("/metrics", promhttp.Handler())

		metricsAddr := fmt.Sprintf("%s:%d", cfg.MetricsHost, cfg.MetricsPort)
		metricsServer = &http.Server{
			Addr:    metricsAddr,
			Handler: metricsMux,
		}

		go func() {
			logger.Info("Metrics server listening", "addr", metricsAddr)
			if err := metricsServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Error("Metrics server failed", "error", err)
			}
		}()
	}

	// Start HTTP server in background
	go func() {
		logger.Info("HTTP server listening", "addr", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)
	<-sigChan

	logger.Info("Shutting down gracefully...")

	// Stop worker
	workerCancel()

	// Shutdown HTTP servers with timeout
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("HTTP server shutdown failed", "error", err)
	}

	if metricsServer != nil {
		if err := metricsServer.Shutdown(shutdownCtx); err != nil {
			logger.Error("Metrics server shutdown failed", "error", err)
		}
	}

	logger.Info("Server stopped")
}
