package main

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"example.com/edtech-device-dashboard/internal/dashboard"
)

const channel = "course-delivery-devices"

type service struct {
	infrai *dashboard.Client
	now    func() time.Time
}

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}
	baseURL := os.Getenv("INFRAI_BASE_URL")
	if baseURL == "" {
		baseURL = dashboard.DefaultBaseURL
	}
	client := dashboard.NewClient(baseURL, key)
	if err := client.CreateChannel(context.Background(), channel, "channel-"+channel); err != nil {
		log.Fatal(err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := client.DeleteChannel(ctx, channel); err != nil {
			log.Printf("delete realtime channel %s: %v", channel, err)
		}
	}()

	svc := service{infrai: client, now: time.Now}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /device-status", svc.deviceStatus)
	mux.HandleFunc("POST /dashboard-token", svc.dashboardToken)
	mux.HandleFunc("GET /dashboard-metrics", svc.dashboardMetrics)

	addr := envOr("ADDR", ":8080")
	log.Printf("device dashboard listening on %s", addr)
	server := &http.Server{Addr: addr, Handler: mux}
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownCtx.Done()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			log.Printf("shut down device dashboard: %v", err)
		}
	}()
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Printf("serve device dashboard: %v", err)
	}
}

func (s service) deviceStatus(w http.ResponseWriter, r *http.Request) {
	var status dashboard.DeviceStatus
	if err := json.NewDecoder(r.Body).Decode(&status); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON body"})
		return
	}
	if status.DeviceID == "" || status.LearnerID == "" || status.CourseID == "" || status.Deadline.IsZero() || status.ObservedAt.IsZero() || status.ProgressPct < 0 || status.ProgressPct > 100 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "device_id, learner_id, course_id, deadline, observed_at, and progress_pct from 0 to 100 are required"})
		return
	}
	decision := dashboard.AssessDelivery(status, s.now())
	idempotencyKey := strings.Join([]string{"status", status.DeviceID, status.ObservedAt.UTC().Format(time.RFC3339Nano)}, ":")
	if err := s.infrai.PublishStatus(r.Context(), channel, status, decision, idempotencyKey); err != nil {
		writeInfraiError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, decision)
}

func (s service) dashboardToken(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ClientID string `json:"client_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || input.ClientID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "client_id is required"})
		return
	}
	token, err := s.infrai.IssueDashboardToken(r.Context(), input.ClientID, channel)
	if err != nil {
		writeInfraiError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, token)
}

func (s service) dashboardMetrics(w http.ResponseWriter, r *http.Request) {
	metrics, err := s.infrai.QueryMetrics(r.Context())
	if err != nil {
		writeInfraiError(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(metrics)
}

func writeInfraiError(w http.ResponseWriter, err error) {
	var apiErr *dashboard.InfraiError
	if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
		writeJSON(w, apiErr.HTTPStatus, map[string]string{"error": apiErr.Message, "code": apiErr.Code})
		return
	}
	writeJSON(w, http.StatusBadGateway, map[string]string{"error": "upstream request failed"})
}

func writeJSON(w http.ResponseWriter, status int, value interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(value)
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
