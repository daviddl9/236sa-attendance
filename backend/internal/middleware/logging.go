package middleware

import (
	"log"
	"net/http"
	"time"
)

// responseWriter wraps http.ResponseWriter to capture status code and response size
type responseWriter struct {
	http.ResponseWriter
	statusCode int
	size       int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Write(b []byte) (int, error) {
	size, err := rw.ResponseWriter.Write(b)
	rw.size += size
	return size, err
}

// Flush implements http.Flusher interface for SSE support
func (rw *responseWriter) Flush() {
	if flusher, ok := rw.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Unwrap returns the underlying ResponseWriter for http.ResponseController
func (rw *responseWriter) Unwrap() http.ResponseWriter {
	return rw.ResponseWriter
}

// RequestResponseLogger logs detailed request and response information
func RequestResponseLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()

		// Wrap response writer to capture status code and size
		wrapped := &responseWriter{
			ResponseWriter: w,
			statusCode:     http.StatusOK, // Default status code
		}

		// Get request ID from context if available
		requestID := r.Header.Get("X-Request-ID")
		if requestID == "" {
			requestID = "-"
		}

		// Get real IP
		realIP := r.Header.Get("X-Real-IP")
		if realIP == "" {
			realIP = r.RemoteAddr
		}

		// Log request
		log.Printf("[REQUEST] %s %s %s | IP: %s | User-Agent: %s | RequestID: %s",
			r.Method,
			r.URL.Path,
			r.URL.RawQuery,
			realIP,
			r.UserAgent(),
			requestID,
		)

		// Process request
		next.ServeHTTP(wrapped, r)

		// Calculate duration
		duration := time.Since(start)

		// Log response
		statusColor := ""
		if wrapped.statusCode >= 500 {
			statusColor = "ERROR"
		} else if wrapped.statusCode >= 400 {
			statusColor = "WARN"
		}

		if statusColor != "" {
			log.Printf("[RESPONSE] %s %s | Status: %d %s | Size: %d bytes | Duration: %v | RequestID: %s",
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				http.StatusText(wrapped.statusCode),
				wrapped.size,
				duration,
				requestID,
			)
		} else {
			log.Printf("[RESPONSE] %s %s | Status: %d | Size: %d bytes | Duration: %v | RequestID: %s",
				r.Method,
				r.URL.Path,
				wrapped.statusCode,
				wrapped.size,
				duration,
				requestID,
			)
		}
	})
}
