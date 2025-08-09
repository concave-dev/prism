package handlers

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// TestHandleHealth tests the health handler response
func TestHandleHealth(t *testing.T) {
	gin.SetMode(gin.TestMode)

	version := "1.0.0"
	startTime := time.Now().Add(-30 * time.Minute) // 30 minutes ago

	handler := HandleHealth(version, startTime)

	// Create test request
	router := gin.New()
	router.GET("/health", handler)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Check status code
	if w.Code != http.StatusOK {
		t.Errorf("HandleHealth() status = %d, want %d", w.Code, http.StatusOK)
	}

	// Parse response
	var response HealthResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check response fields
	if response.Status != "healthy" {
		t.Errorf("HandleHealth() status = %q, want \"healthy\"", response.Status)
	}

	if response.Version != version {
		t.Errorf("HandleHealth() version = %q, want %q", response.Version, version)
	}

	// Check that timestamp is recent (within last 5 seconds)
	if time.Since(response.Timestamp) > 5*time.Second {
		t.Error("HandleHealth() timestamp is not recent")
	}

	// Check that uptime is reasonable (should be around 30 minutes)
	if response.Uptime == "" {
		t.Error("HandleHealth() uptime is empty")
	}
}
