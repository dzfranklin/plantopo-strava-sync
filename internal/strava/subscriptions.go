package strava

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
)

// SubscriptionRequest represents a webhook subscription request
type SubscriptionRequest struct {
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	CallbackURL  string `json:"callback_url"`
	VerifyToken  string `json:"verify_token"`
}

// Subscription represents a Strava webhook subscription
type Subscription struct {
	ID            int    `json:"id"`
	ApplicationID int    `json:"application_id"`
	CallbackURL   string `json:"callback_url"`
	CreatedAt     string `json:"created_at"`
	UpdatedAt     string `json:"updated_at"`
}

// CreateSubscription creates a new webhook subscription
// Note: This does not require athlete authentication, only app credentials
func (c *Client) CreateSubscription(callbackURL, verifyToken, clientID string) (*Subscription, error) {
	clientConfig, err := c.config.GetClient(clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid client: %w", err)
	}

	data := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
		"callback_url":  {callbackURL},
		"verify_token":  {verifyToken},
	}

	resp, err := c.httpClient.PostForm(c.baseURL+"/push_subscriptions", data)
	if err != nil {
		return nil, fmt.Errorf("failed to create subscription: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var subscription Subscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("failed to decode subscription response: %w", err)
	}

	return &subscription, nil
}

// ListSubscriptions lists all active webhook subscriptions for this application
func (c *Client) ListSubscriptions(clientID string) ([]*Subscription, error) {
	clientConfig, err := c.config.GetClient(clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid client: %w", err)
	}

	params := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
	}

	reqURL := c.baseURL + "/push_subscriptions?" + params.Encode()
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscriptions: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var subscriptions []*Subscription
	if err := json.Unmarshal(body, &subscriptions); err != nil {
		return nil, fmt.Errorf("failed to decode subscriptions response: %w", err)
	}

	return subscriptions, nil
}

// DeleteSubscription deletes a webhook subscription
func (c *Client) DeleteSubscription(subscriptionID int, clientID string) error {
	clientConfig, err := c.config.GetClient(clientID)
	if err != nil {
		return fmt.Errorf("invalid client: %w", err)
	}

	params := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
	}

	reqURL := fmt.Sprintf("%s/push_subscriptions/%d?%s", c.baseURL, subscriptionID, params.Encode())
	req, err := http.NewRequest("DELETE", reqURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to delete subscription: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	return nil
}

// ViewSubscription retrieves details about a specific subscription
func (c *Client) ViewSubscription(subscriptionID int, clientID string) (*Subscription, error) {
	clientConfig, err := c.config.GetClient(clientID)
	if err != nil {
		return nil, fmt.Errorf("invalid client: %w", err)
	}

	params := url.Values{
		"client_id":     {clientConfig.ClientID},
		"client_secret": {clientConfig.ClientSecret},
	}

	reqURL := fmt.Sprintf("%s/push_subscriptions/%d?%s", c.baseURL, subscriptionID, params.Encode())
	req, err := http.NewRequest("GET", reqURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to view subscription: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		return nil, &HTTPError{
			StatusCode: resp.StatusCode,
			Body:       string(body),
		}
	}

	var subscription Subscription
	if err := json.Unmarshal(body, &subscription); err != nil {
		return nil, fmt.Errorf("failed to decode subscription response: %w", err)
	}

	return &subscription, nil
}
