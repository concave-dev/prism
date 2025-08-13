// Package api provides HTTP middleware components for the Prism cluster management API.
//
// This file implements essential middleware layers that handle cross-cutting concerns
// for all HTTP requests processed by the API server. The middleware stack provides:
//   - Request/response logging with structured output for observability
//   - CORS (Cross-Origin Resource Sharing) headers for web client compatibility
//   - Standardized error handling and response formatting
//
// Middleware functions are designed to be composable and are applied in order during
// request processing. The logging middleware captures request details, timing, and
// status codes for monitoring and debugging. The CORS middleware enables web-based
// clients and tools to interact with the API from different origins, which is essential
// for browser-based cluster management interfaces and development workflows.
//
// All middleware integrates with Prism's structured logging system to maintain
// consistent log formatting across the entire cluster management platform.

package api

import (
	"time"

	"github.com/concave-dev/prism/internal/logging"
	"github.com/gin-gonic/gin"
)

// loggingMiddleware creates a Gin middleware handler that logs all HTTP requests
// using Prism's structured logging system.
//
// This middleware captures comprehensive request information including client IP,
// timestamps, HTTP method/path, response status codes, processing latency, and
// user agent strings. All logged data follows a standardized format for easier
// parsing by log aggregation tools and monitoring systems.
//
// The middleware is essential for cluster operations because it provides visibility
// into API usage patterns, performance characteristics, and potential issues across
// all nodes in the distributed system. This logging data is crucial for debugging
// cluster-wide problems and monitoring the health of the management API.
func (s *Server) loggingMiddleware() gin.HandlerFunc {
	return gin.LoggerWithFormatter(func(param gin.LogFormatterParams) string {
		// Log using our custom logger
		logging.Info("%s - [%s] \"%s %s %s %d %s \"%s\" %s\"",
			param.ClientIP,
			param.TimeStamp.Format(time.RFC1123),
			param.Method,
			param.Path,
			param.Request.Proto,
			param.StatusCode,
			param.Latency,
			param.Request.UserAgent(),
			param.ErrorMessage,
		)
		return ""
	})
}

// corsMiddleware creates a Gin middleware handler that adds Cross-Origin Resource
// Sharing (CORS) headers to all HTTP responses.
//
// This middleware enables web browsers and other HTTP clients to make requests to
// the Prism API from different origins (domains, ports, or protocols). It sets
// permissive CORS headers allowing all origins, common HTTP methods, and standard
// request headers. The middleware also handles preflight OPTIONS requests that
// browsers send before actual cross-origin requests.
//
// CORS support is essential for web-based cluster management tools, browser-based
// debugging interfaces, and development workflows where the frontend and API may
// be served from different ports or domains. Without proper CORS headers, modern
// browsers would block these cross-origin requests for security reasons.
func (s *Server) corsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-CSRF-Token")
		c.Header("Access-Control-Expose-Headers", "Link")
		c.Header("Access-Control-Allow-Credentials", "true")
		c.Header("Access-Control-Max-Age", "300")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}

		c.Next()
	}
}
