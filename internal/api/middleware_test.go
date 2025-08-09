package api

import (
	"net/http/httptest"
	"testing"

	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// TestCORSMiddleware tests CORS header setting
func TestCORSMiddleware(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)

	// Create router with CORS middleware
	router := gin.New()
	router.Use(server.corsMiddleware())
	router.GET("/test", func(c *gin.Context) {
		c.JSON(200, gin.H{"message": "ok"})
	})

	tests := []struct {
		name           string
		method         string
		expectedStatus int
		checkHeaders   bool
	}{
		{
			name:           "GET request with CORS headers",
			method:         "GET",
			expectedStatus: 200,
			checkHeaders:   true,
		},
		{
			name:           "OPTIONS request should return 204",
			method:         "OPTIONS",
			expectedStatus: 204,
			checkHeaders:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/test", nil)
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if tt.checkHeaders {
				// Test CORS headers
				expectedHeaders := map[string]string{
					"Access-Control-Allow-Origin":      "*",
					"Access-Control-Allow-Methods":     "GET, POST, PUT, DELETE, OPTIONS",
					"Access-Control-Allow-Headers":     "Accept, Authorization, Content-Type, X-CSRF-Token",
					"Access-Control-Expose-Headers":    "Link",
					"Access-Control-Allow-Credentials": "true",
					"Access-Control-Max-Age":           "300",
				}

				for header, expectedValue := range expectedHeaders {
					actualValue := w.Header().Get(header)
					if actualValue != expectedValue {
						t.Errorf("Header %s = %q, want %q", header, actualValue, expectedValue)
					}
				}
			}
		})
	}
}

// TestCORSMiddleware_OptionsMethod tests that OPTIONS requests are handled correctly
func TestCORSMiddleware_OptionsMethod(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)

	router := gin.New()
	router.Use(server.corsMiddleware())

	// Add a handler that should NOT be called for OPTIONS
	handlerCalled := false
	router.OPTIONS("/test", func(c *gin.Context) {
		handlerCalled = true
		c.JSON(200, gin.H{"message": "should not be called"})
	})

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// OPTIONS should return 204 and abort, not call the handler
	if w.Code != 204 {
		t.Errorf("OPTIONS request status = %d, want 204", w.Code)
	}

	if handlerCalled {
		t.Error("OPTIONS request should abort before reaching handler")
	}
}

// TestLoggingMiddleware_NonNil tests that logging middleware returns a valid function
func TestLoggingMiddleware_NonNil(t *testing.T) {
	config := &Config{
		BindAddr:    "127.0.0.1",
		BindPort:    8080,
		SerfManager: &serf.SerfManager{},
		RaftManager: &raft.RaftManager{},
	}

	server := NewServer(config)
	middleware := server.loggingMiddleware()

	if middleware == nil {
		t.Error("loggingMiddleware() returned nil")
	}
}
