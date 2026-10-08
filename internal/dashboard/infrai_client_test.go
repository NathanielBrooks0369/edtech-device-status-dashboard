package dashboard

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestClientReturnsEnvelopeErrorBeforeHTTPStatus(t *testing.T) {
	client := NewClient("https://example.test", "test-key")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusBadRequest,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":false,"data":null,"error":{"message":"device id is required"},"metadata":null}`)),
			Request:    req,
		}, nil
	})}

	err := client.PublishStatus(context.Background(), "course-delivery", DeviceStatus{LearnerID: "learner-7"}, DeliveryDecision{}, "status-1")
	var apiErr *InfraiError
	if !errors.As(err, &apiErr) {
		t.Fatalf("expected InfraiError, got %T: %v", err, err)
	}
	if apiErr.Message != "device id is required" || apiErr.HTTPStatus != http.StatusBadRequest {
		t.Fatalf("unexpected error: %#v", apiErr)
	}
}

func TestDeleteChannel(t *testing.T) {
	client := NewClient("https://example.test", "test-key")
	client.HTTP = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Method != http.MethodDelete || req.URL.Path != "/v1/realtime/channel/delete/course-delivery-devices" {
			t.Fatalf("unexpected request: %s %s", req.Method, req.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(strings.NewReader(`{"ok":true,"data":null,"error":null,"metadata":null}`)),
			Request:    req,
		}, nil
	})}

	if err := client.DeleteChannel(context.Background(), "course-delivery-devices"); err != nil {
		t.Fatalf("DeleteChannel() error = %v", err)
	}
}
