package api

import (
	"net"
	"net/http/httptest"
	"testing"

	"github.com/concave-dev/prism/internal/api/batching"
	"github.com/concave-dev/prism/internal/grpc"
	"github.com/concave-dev/prism/internal/raft"
	"github.com/concave-dev/prism/internal/serf"
	"github.com/gin-gonic/gin"
)

// TestCORSMiddleware tests CORS header setting
func TestCORSMiddleware(t *testing.T) {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:       "127.0.0.1",
		BindPort:       8080,
		BatchingConfig: batching.DefaultConfig(),
		SerfManager:    &serf.SerfManager{},
		RaftManager:    &raft.RaftManager{},
		GRPCClientPool: &grpc.ClientPool{},
	}

	// Create a mock listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create test listener: %v", err)
	}
	defer listener.Close()

	server, err := NewServerWithListener(config, listener)
	if err != nil {
		t.Fatalf("NewServerWithListener() error = %v", err)
	}

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

// TestCORSMiddleware_OptionsHandling tests OPTIONS request handling
func TestCORSMiddleware_OptionsHandling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	config := &Config{
		BindAddr:       "127.0.0.1",
		BindPort:       8080,
		BatchingConfig: batching.DefaultConfig(),
		SerfManager:    &serf.SerfManager{},
		RaftManager:    &raft.RaftManager{},
		GRPCClientPool: &grpc.ClientPool{},
	}

	// Create a mock listener
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Failed to create test listener: %v", err)
	}
	defer listener.Close()

	server, err := NewServerWithListener(config, listener)
	if err != nil {
		t.Fatalf("NewServerWithListener() error = %v", err)
	}

	router := gin.New()
	router.Use(server.corsMiddleware())

	req := httptest.NewRequest("OPTIONS", "/test", nil)
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// OPTIONS should return 204 and set CORS headers
	if w.Code != 204 {
		t.Errorf("OPTIONS request status = %d, want 204", w.Code)
	}

	// Check key CORS header is set
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("Access-Control-Allow-Origin header not set correctly")
	}
}
