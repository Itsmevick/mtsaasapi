package middleware

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/google/uuid"
	"github.com/multitenant-saas/api/internal/config"
	"github.com/multitenant-saas/api/internal/database"
	"github.com/multitenant-saas/api/internal/services"
	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog/log"
)

type Middleware struct {
	authService *services.AuthService
	config      *config.Config
	redis       *redis.Client
}

func New(authService *services.AuthService, cfg *config.Config, redisClient *redis.Client) *Middleware {
	return &Middleware{
		authService: authService,
		config:      cfg,
		redis:       redisClient,
	}
}

// RequireAuth ensures the request is authenticated
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get session token from cookie
		cookie, err := r.Cookie("session")
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Get user from session
		user, err := m.authService.GetSession(r.Context(), cookie.Value)
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Add user to context
		ctx := database.WithUser(r.Context(), user.ID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireWorkspace ensures the request has a workspace context
func (m *Middleware) RequireWorkspace(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get workspace ID from header or query param
		workspaceIDStr := r.Header.Get("X-Workspace-ID")
		if workspaceIDStr == "" {
			workspaceIDStr = r.URL.Query().Get("workspace_id")
		}

		// Trim whitespace
		workspaceIDStr = strings.TrimSpace(workspaceIDStr)

		if workspaceIDStr == "" {
			log.Debug().Msg("Workspace ID header missing or empty")
			http.Error(w, "Workspace ID required", http.StatusBadRequest)
			return
		}

		workspaceID, err := uuid.Parse(workspaceIDStr)
		if err != nil {
			http.Error(w, "Invalid workspace ID", http.StatusBadRequest)
			return
		}

		// Verify user is member of workspace
		userID, err := database.GetUserID(r.Context())
		if err != nil {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Check membership (simplified - would use member service)
		var memberID uuid.UUID
		err = m.authService.GetDB().QueryRow(r.Context(),
			"SELECT id FROM workspace_members WHERE workspace_id = $1 AND user_id = $2",
			workspaceID, userID,
		).Scan(&memberID)
		if err != nil {
			log.Debug().Err(err).Msg("Workspace membership check failed")
			http.Error(w, "Access denied", http.StatusForbidden)
			return
		}

		// Add tenant to context
		ctx := database.WithTenant(r.Context(), workspaceID)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireRole ensures the user has the required role in the workspace
func (m *Middleware) RequireRole(requiredRole string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			workspaceID, err := database.GetTenantID(r.Context())
			if err != nil {
				http.Error(w, "Workspace context required", http.StatusBadRequest)
				return
			}

			userID, err := database.GetUserID(r.Context())
			if err != nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Get user's role in workspace
			var role string
			err = m.authService.GetDB().QueryRow(r.Context(),
				"SELECT role FROM workspace_members WHERE workspace_id = $1 AND user_id = $2",
				workspaceID, userID,
			).Scan(&role)
			if err != nil {
				http.Error(w, "Access denied", http.StatusForbidden)
				return
			}

			// Check role hierarchy: owner > admin > member
			roleHierarchy := map[string]int{
				"owner":  3,
				"admin":  2,
				"member": 1,
			}

			userLevel := roleHierarchy[role]
			requiredLevel := roleHierarchy[requiredRole]

			if userLevel < requiredLevel {
				http.Error(w, "Insufficient permissions", http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// CORS middleware
func (m *Middleware) CORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		allowedOrigin := m.config.CORSOrigin
		isDevelopment := m.config.Env == "development"

		// In production: exact match required (no wildcard)
		// In development: allow localhost:3000
		if isDevelopment {
			// Development: allow http://localhost:3000
			if origin == "http://localhost:3000" {
				w.Header().Set("Access-Control-Allow-Origin", "http://localhost:3000")
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Workspace-ID")
			}
		} else {
			// Production: exact match with CORS_ORIGIN (must be exactly https://mtsaasweb.vercel.app or configured value)
			if origin == allowedOrigin {
				w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
				w.Header().Set("Access-Control-Allow-Credentials", "true")
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, X-Workspace-ID")
			} else {
				// Log mismatch for debugging
				log.Warn().Str("request_origin", origin).Str("allowed_origin", allowedOrigin).Msg("CORS origin mismatch in production")
			}
		}

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// Logging middleware with structured request logging
func (m *Middleware) Logging(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		
		// Get request ID from Chi middleware
		requestID := middleware.GetReqID(r.Context())
		if requestID == "" {
			requestID = "unknown"
		}
		
		// Create response writer wrapper to capture status code
		ww := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		
		// Process request
		next.ServeHTTP(ww, r)
		
		// Calculate latency
		latency := time.Since(start)
		
		// Build structured log entry
		logEntry := log.Info().
			Str("request_id", requestID).
			Str("method", r.Method).
			Str("route", r.URL.Path).
			Str("ip", getClientIP(r)).
			Int("status", ww.statusCode).
			Dur("latency_ms", latency).
			Str("user_agent", r.UserAgent())
		
		// Add workspace ID if available (but not sensitive data)
		if workspaceID := r.Header.Get("X-Workspace-ID"); workspaceID != "" {
			logEntry = logEntry.Str("workspace_id", workspaceID)
		}
		
		// Log based on status code
		if ww.statusCode >= 500 {
			logEntry.Msg("Request completed (server error)")
		} else if ww.statusCode >= 400 {
			logEntry.Msg("Request completed (client error)")
		} else {
			logEntry.Msg("Request completed")
		}
	})
}

// Recoverer middleware with structured error logging
func (m *Middleware) Recoverer(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if err := recover(); err != nil {
				// Get request ID for correlation
				requestID := middleware.GetReqID(r.Context())
				if requestID == "" {
					requestID = "unknown"
				}
				
				// Get stack trace
				stack := make([]byte, 4096)
				stack = stack[:runtime.Stack(stack, false)]
				
				// Log error with full context
				log.Error().
					Str("request_id", requestID).
					Str("method", r.Method).
					Str("route", r.URL.Path).
					Str("ip", getClientIP(r)).
					Interface("error", err).
					Bytes("stack", stack).
					Msg("Panic recovered")
				
				// Return 500 error
				w.WriteHeader(http.StatusInternalServerError)
				w.Write([]byte(`{"error": "Internal server error"}`))
			}
		}()
		
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.statusCode = code
	rw.ResponseWriter.WriteHeader(code)
}

// SecurityHeaders adds standard security headers to responses
func (m *Middleware) SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")
		
		// Prevent clickjacking
		w.Header().Set("X-Frame-Options", "DENY")
		
		// Enable XSS protection (legacy but still useful)
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		
		// Referrer policy
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		
		// Permissions policy (formerly Feature-Policy)
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		
		// HSTS (only in production with HTTPS)
		if m.config.Env == "production" {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains; preload")
		}
		
		next.ServeHTTP(w, r)
	})
}

// RateLimit applies rate limiting based on route and IP address
func (m *Middleware) RateLimit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get client IP
		ip := getClientIP(r)
		
		// Determine rate limit based on route
		var limit int
		var window time.Duration
		var keyPrefix string
		
		path := r.URL.Path
		method := r.Method
		
		// Strict limit for /auth/login (10 requests per minute)
		if path == "/api/v1/auth/login" {
			limit = 10
			window = 1 * time.Minute
			keyPrefix = "ratelimit:login"
		} else if method == "POST" || method == "PATCH" || method == "DELETE" {
			// Moderate limit for write operations (60 requests per minute)
			limit = 60
			window = 1 * time.Minute
			keyPrefix = "ratelimit:write"
		} else {
			// No rate limit for GET requests (except login)
			next.ServeHTTP(w, r)
			return
		}
		
		// Create Redis key
		key := fmt.Sprintf("%s:%s", keyPrefix, ip)
		
		// Check current count
		ctx := r.Context()
		count, err := m.redis.Get(ctx, key).Int()
		if err == redis.Nil {
			// First request, set count to 1 with expiration
			m.redis.Set(ctx, key, 1, window)
			next.ServeHTTP(w, r)
			return
		} else if err != nil {
			// Redis error - log but allow request (fail open)
			log.Warn().Err(err).Msg("Rate limit check failed, allowing request")
			next.ServeHTTP(w, r)
			return
		}
		
		// Check if limit exceeded
		if count >= limit {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(window.Seconds())))
			w.WriteHeader(http.StatusTooManyRequests)
			w.Write([]byte(`{"error": "Rate limit exceeded. Please try again later."}`))
			return
		}
		
		// Increment counter
		m.redis.Incr(ctx, key)
		// Reset expiration if this is a new window
		m.redis.Expire(ctx, key, window)
		
		next.ServeHTTP(w, r)
	})
}

// getClientIP extracts the client IP from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header (for proxies/load balancers)
	forwarded := r.Header.Get("X-Forwarded-For")
	if forwarded != "" {
		// Take the first IP (client IP)
		ips := strings.Split(forwarded, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}
	
	// Check X-Real-IP header
	realIP := r.Header.Get("X-Real-IP")
	if realIP != "" {
		return realIP
	}
	
	// Fallback to RemoteAddr
	ip := r.RemoteAddr
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
