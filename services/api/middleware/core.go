package middleware

import (
	"context"
	"net/http"
	"strings"
)

// UserContextKey is the context key for the user ID.
type UserContextKey struct{}

// StubAuthMiddleware checks for a simple Authorization header or creates a dummy user.
// In a real system, this would validate JWTs or check sessions.
func StubAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			// For development, allow specific non-auth routes or fail
			// Just failing by default for "/api/v1" routes that aren't public
			if strings.HasPrefix(r.URL.Path, "/api/v1/ping") {
				next.ServeHTTP(w, r)
				return
			}
			// Allow "Bearer stub" or just "stub"
		}

		// Mock User ID: 1 (Superuser)
		ctx := context.WithValue(r.Context(), UserContextKey{}, int64(1))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// LoggerMiddleware is a basic request logger (using standard lib or simple print for now).
func LoggerMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// In a real app, use structured logging (zap/zerolog)
		// fmt.Printf("Request: %s %s\n", r.Method, r.URL.Path)
		next.ServeHTTP(w, r)
	})
}
