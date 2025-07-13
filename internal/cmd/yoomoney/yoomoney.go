package yoomoney

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"paymentgo/internal/config"
	dto "paymentgo/internal/entity"
)

type Client struct {
	httpClient *http.Client
	authToken  string
	clientID   string
	baseURL    string
}

func New(cfg *config.Config) *Client {
	return &Client{
		httpClient: &http.Client{Timeout: 10 * time.Second},
		authToken:  cfg.Yoomoney.Token,
		clientID:   cfg.Yoomoney.ClientID,
		baseURL:    "https://yoomoney.ru",
	}
}

// CheckTransactionStatus fetches the latest status of a payment operation based on its label.
func (c *Client) CheckTransactionStatus(label string) (string, error) {
	endpoint := fmt.Sprintf("%s/api/operation-history", c.baseURL)

	data := url.Values{}
	data.Set("label", label)
	data.Set("records", "1")
	data.Set("type", "deposition")

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return "error", fmt.Errorf("could not build request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "error", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "error", fmt.Errorf("error reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "error", fmt.Errorf("unexpected status: %s — %s", resp.Status, string(raw))
	}

	var parsed struct {
		Error      string `json:"error"`
		Operations []struct {
			Status string `json:"status"`
		} `json:"operations"`
	}

	if err := json.Unmarshal(raw, &parsed); err != nil {
		return "error", fmt.Errorf("invalid JSON structure: %w", err)
	}

	if parsed.Error != "" {
		return "error", fmt.Errorf("API error: %s", parsed.Error)
	}

	if len(parsed.Operations) == 0 {
		return "error", fmt.Errorf("no transactions found for label: %s", label)
	}

	switch parsed.Operations[0].Status {
	case "success":
		return "success", nil
	case "refused":
		return "failed", fmt.Errorf("transaction was refused")
	case "in_progress":
		return "pending", nil
	default:
		return "error", fmt.Errorf("unrecognized status: %s", parsed.Operations[0].Status)
	}
}

// InitiateTransfer starts a payment request to a specific recipient.
func (c *Client) InitiateTransfer(payment *dto.Payment, recipient string) (string, error) {
	if payment == nil {
		return "", fmt.Errorf("payment details cannot be nil")
	}
	if payment.ToUserID == "" || payment.ID == "" || payment.Currency == "" || payment.Amount <= 0 {
		return "", fmt.Errorf("invalid payment fields: %+v", payment)
	}

	endpoint := fmt.Sprintf("%s/api/request-payment", c.baseURL)

	payload := url.Values{}
	payload.Set("pattern_id", "p2p")
	payload.Set("to", recipient)
	payload.Set("amount", strconv.FormatFloat(payment.Amount, 'f', 2, 64))
	payload.Set("comment", payment.ID)
	payload.Set("message", payment.ID)
	payload.Set("label", payment.ID)
	payload.Set("currency", payment.Currency)

	req, err := http.NewRequest("POST", endpoint, strings.NewReader(payload.Encode()))
	if err != nil {
		return "", fmt.Errorf("could not create request: %w", err)
	}

	req.Header.Set("Authorization", "Bearer "+c.authToken)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("API call failed: %w", err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("response read error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API returned %s: %s", resp.Status, string(raw))
	}

	var result map[string]interface{}
	if err := json.Unmarshal(raw, &result); err != nil {
		return "", fmt.Errorf("response parsing error: %w", err)
	}

	status, ok := result["status"].(string)
	if !ok {
		return "", fmt.Errorf("response missing 'status'")
	}

	switch status {
	case "success":
		return "success", nil
	case "refused":
		msg, _ := result["error"].(string)
		return "failed", fmt.Errorf("transfer refused: %s", msg)
	default:
		return "error", fmt.Errorf("unexpected status: %s", status)
	}
}

// GenerateQuickPayURL constructs a quick payment URL with optional parameters.
func (c *Client) GenerateQuickPayURL(receiver, target, paymentType string, amount float64, formComment, label, comment, redirectURL string) (string, error) {
	if receiver == "" || amount <= 0 {
		return "", fmt.Errorf("receiver and amount must be valid")
	}

	endpoint := "https://yoomoney.ru/quickpay/confirm?"

	params := url.Values{}
	params.Set("receiver", receiver)
	params.Set("quickpay-form", "shop")
	params.Set("paymentType", paymentType)
	params.Set("sum", strconv.FormatFloat(amount, 'f', 2, 64))
	params.Set("targets", target)

	if formComment != "" {
		params.Set("formcomment", formComment)
	}
	if label != "" {
		params.Set("label", label)
	}
	if comment != "" {
		params.Set("comment", comment)
	}
	if redirectURL != "" {
		params.Set("successURL", redirectURL)
	}

	fullURL := endpoint + params.Encode()

	resp, err := http.Get(fullURL)
	if err != nil {
		return "", fmt.Errorf("URL validation failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return fullURL, nil
	}

	return "", fmt.Errorf("unexpected response status: %s", resp.Status)
}
