package main

import (
	"fmt"
	"log/slog"
	"os"

	"plantopo-strava-sync/internal/config"
	"plantopo-strava-sync/internal/database"
	"plantopo-strava-sync/internal/strava"
)

func main() {
	// Disable structured logging for CLI
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: slog.LevelError, // Only show errors
	})))

	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

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

	switch command {
	case "subscribe":
		handleSubscribe(client, cfg)
	case "list":
		handleList(client)
	case "view":
		handleView(client)
	case "unsubscribe":
		handleUnsubscribe(client)
	case "help":
		printUsage()
	default:
		fmt.Fprintf(os.Stderr, "Error: Unknown command '%s'\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println(`plantopo-strava-sync CLI - Webhook Subscription Management

Usage:
  cli <command> [options]

Commands:
  subscribe    Create a new webhook subscription
  list         List all active subscriptions
  view [id]    View details of a specific subscription
  unsubscribe [id]  Delete a webhook subscription
  help         Show this help message

Examples:
  cli subscribe
  cli list
  cli view 12345
  cli unsubscribe 12345

Environment Variables Required:
  STRAVA_CLIENT_ID       - Strava application client ID
  STRAVA_CLIENT_SECRET   - Strava application client secret
  STRAVA_VERIFY_TOKEN    - Token for webhook verification
  HOST                   - Server host (default: localhost)
  PORT                   - Server port (default: 4101)`)
}

func handleSubscribe(client *strava.Client, cfg *config.Config) {
	callbackURL := fmt.Sprintf("http://%s:%d/webhook-callback", cfg.Host, cfg.Port)

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

func handleList(client *strava.Client) {
	fmt.Println("Fetching subscriptions...")

	subscriptions, err := client.ListSubscriptions()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: Failed to list subscriptions: %v\n", err)
		os.Exit(1)
	}

	if len(subscriptions) == 0 {
		fmt.Println("No active subscriptions found.")
		fmt.Println("\nTo create a subscription, run: cli subscribe")
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

func handleView(client *strava.Client) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Error: Subscription ID required")
		fmt.Fprintln(os.Stderr, "Usage: cli view <subscription_id>")
		os.Exit(1)
	}

	var subscriptionID int
	if _, err := fmt.Sscanf(os.Args[2], "%d", &subscriptionID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid subscription ID: %s\n", os.Args[2])
		os.Exit(1)
	}

	fmt.Printf("Fetching subscription %d...\n", subscriptionID)

	subscription, err := client.ViewSubscription(subscriptionID)
	if err != nil {
		if httpErr, ok := err.(*strava.HTTPError); ok && httpErr.StatusCode == 404 {
			fmt.Fprintf(os.Stderr, "Error: Subscription %d not found\n", subscriptionID)
		} else {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("\nSubscription Details:")
	fmt.Printf("  ID: %d\n", subscription.ID)
	fmt.Printf("  Application ID: %d\n", subscription.ApplicationID)
	fmt.Printf("  Callback URL: %s\n", subscription.CallbackURL)
	fmt.Printf("  Created At: %s\n", subscription.CreatedAt)
	fmt.Printf("  Updated At: %s\n", subscription.UpdatedAt)
}

func handleUnsubscribe(client *strava.Client) {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "Error: Subscription ID required")
		fmt.Fprintln(os.Stderr, "Usage: cli unsubscribe <subscription_id>")
		os.Exit(1)
	}

	var subscriptionID int
	if _, err := fmt.Sscanf(os.Args[2], "%d", &subscriptionID); err != nil {
		fmt.Fprintf(os.Stderr, "Error: Invalid subscription ID: %s\n", os.Args[2])
		os.Exit(1)
	}

	fmt.Printf("Deleting subscription %d...\n", subscriptionID)

	err := client.DeleteSubscription(subscriptionID)
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
