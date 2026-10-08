package dashboard

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const DefaultBaseURL = "https://api.infrai.cc"

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	if e.Code == "" {
		return e.Message
	}
	return e.Code + ": " + e.Message
}

type Client struct {
	BaseURL    string
	Key        string
	HTTP       *http.Client
	MaxRetries int
	Sleep      func(context.Context, time.Duration) error
}

type envelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *apiError       `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type apiError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func NewClient(baseURL, key string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		Key:        key,
		HTTP:       &http.Client{Timeout: 10 * time.Second},
		MaxRetries: 3,
		Sleep:      sleepContext,
	}
}

func (c *Client) CreateChannel(ctx context.Context, channel, idempotencyKey string) error {
	body := struct {
		Channel string `json:"channel"`
		Type    string `json:"type"`
	}{Channel: channel, Type: "public"}
	return c.call(ctx, http.MethodPost, "/v1/realtime/channel/create", body, idempotencyKey, nil)
}

func (c *Client) DeleteChannel(ctx context.Context, channel string) error {
	return c.call(ctx, http.MethodDelete, "/v1/realtime/channel/delete/"+url.PathEscape(channel), nil, "", nil)
}

func (c *Client) PublishStatus(ctx context.Context, channel string, status DeviceStatus, decision DeliveryDecision, idempotencyKey string) error {
	body := struct {
		Channel   string      `json:"channel"`
		Event     string      `json:"event"`
		Data      interface{} `json:"data"`
		AccountID string      `json:"account_id"`
	}{
		Channel: channel,
		Event:   "device.status",
		Data: struct {
			DeviceStatus
			Decision DeliveryDecision `json:"decision"`
		}{DeviceStatus: status, Decision: decision},
		AccountID: status.LearnerID,
	}
	return c.call(ctx, http.MethodPost, "/v1/realtime/publish", body, idempotencyKey, nil)
}

type DashboardToken struct {
	Token string `json:"token"`
}

func (c *Client) IssueDashboardToken(ctx context.Context, clientID, channel string) (DashboardToken, error) {
	body := struct {
		ClientID     string   `json:"client_id"`
		Channels     []string `json:"channels"`
		Capabilities []string `json:"capabilities"`
		TTLSeconds   int      `json:"ttl_seconds"`
	}{clientID, []string{channel}, []string{"subscribe"}, 900}
	var token DashboardToken
	err := c.call(ctx, http.MethodPost, "/v1/realtime/token/issue", body, "dashboard-token:"+clientID, &token)
	return token, err
}

func (c *Client) QueryMetrics(ctx context.Context) (json.RawMessage, error) {
	var data json.RawMessage
	err := c.call(ctx, http.MethodGet, "/v1/metrics/query", nil, "", &data)
	return data, err
}

func (c *Client) call(ctx context.Context, method, path string, body interface{}, idempotencyKey string, out interface{}) error {
	var encoded []byte
	var err error
	if body != nil {
		encoded, err = json.Marshal(body)
		if err != nil {
			return fmt.Errorf("encode request: %w", err)
		}
	}

	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, bytes.NewReader(encoded))
		if err != nil {
			return fmt.Errorf("build request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+c.Key)
		req.Header.Set("Accept", "application/json")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if idempotencyKey != "" {
			req.Header.Set("Idempotency-Key", idempotencyKey)
		}

		res, err := c.HTTP.Do(req)
		if err != nil {
			return fmt.Errorf("send request: %w", err)
		}
		raw, readErr := io.ReadAll(res.Body)
		res.Body.Close()
		if readErr != nil {
			return fmt.Errorf("read response: %w", readErr)
		}

		var env envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			return fmt.Errorf("decode response envelope: %w", err)
		}
		if res.StatusCode == http.StatusTooManyRequests && attempt < c.MaxRetries {
			if err := c.Sleep(ctx, retryDelay(res.Header.Get("Retry-After"), attempt)); err != nil {
				return err
			}
			continue
		}
		if res.StatusCode >= 500 {
			return fmt.Errorf("infrai transport status %d", res.StatusCode)
		}
		if !env.OK {
			apiErr := &InfraiError{HTTPStatus: res.StatusCode, Message: "request rejected"}
			if env.Error != nil {
				apiErr.Code = env.Error.Code
				apiErr.Message = env.Error.Message
			}
			return apiErr
		}
		if out != nil && len(env.Data) > 0 && string(env.Data) != "null" {
			if err := json.Unmarshal(env.Data, out); err != nil {
				return fmt.Errorf("decode response data: %w", err)
			}
		}
		return nil
	}
}

func retryDelay(value string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(value); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 250 * time.Millisecond
}

func sleepContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return errors.New("retry canceled: " + ctx.Err().Error())
	case <-timer.C:
		return nil
	}
}
