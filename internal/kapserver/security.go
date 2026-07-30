package kapserver

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// SecurityConfig holds security middleware configuration.
type SecurityConfig struct {
	// AllowedOrigins restricts CORS to specific origins. Empty = allow all.
	AllowedOrigins []string
	// RateLimitPerMinute is the max requests per minute per IP. 0 = no limit.
	RateLimitPerMinute int
	// BindAddress is the address the server is bound to.
	BindAddress string
	// TrustedProxies is the set of proxy IPs whose X-Forwarded-For /
	// X-Real-IP headers should be trusted. When empty, proxy headers are
	// ignored and RemoteAddr is used directly.
	TrustedProxies map[string]bool
}

// SecurityMiddleware provides host classification, origin/CORS validation,
// and rate limiting for non-loopback deployments.
type SecurityMiddleware struct {
	config    SecurityConfig
	isLoopback bool
	rateLimiter *rateLimiter
}

// NewSecurityMiddleware creates a new security middleware.
func NewSecurityMiddleware(cfg SecurityConfig) *SecurityMiddleware {
	isLoopback := isLoopbackAddress(cfg.BindAddress)
	var rl *rateLimiter
	if cfg.RateLimitPerMinute > 0 {
		rl = newRateLimiter(cfg.RateLimitPerMinute)
	}
	return &SecurityMiddleware{
		config:      cfg,
		isLoopback:  isLoopback,
		rateLimiter: rl,
	}
}

// Wrap applies security middleware to an HTTP handler.
func (m *SecurityMiddleware) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Origin/CORS validation
		origin := r.Header.Get("Origin")
		if origin != "" {
			if !m.isAllowedOrigin(origin) {
				http.Error(w, `{"code":40301,"msg":"origin not allowed"}`, http.StatusForbidden)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Vary", "Origin")
		} else {
			w.Header().Set("Access-Control-Allow-Origin", "*")
		}

		// Host header validation for non-loopback
		if !m.isLoopback {
			host := r.Host
			if host == "" {
				host = r.URL.Host
			}
			if !m.isAllowedHost(host) {
				http.Error(w, `{"code":40302,"msg":"host not allowed"}`, http.StatusForbidden)
				return
			}
		}

		// Rate limiting
		if m.rateLimiter != nil {
			clientIP := extractClientIP(r, m.config.TrustedProxies)
			if !m.rateLimiter.Allow(clientIP) {
				w.Header().Set("Retry-After", "60")
				http.Error(w, `{"code":42901,"msg":"rate limit exceeded"}`, http.StatusTooManyRequests)
				return
			}
		}

		// Security headers
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")

		next.ServeHTTP(w, r)
	})
}

func (m *SecurityMiddleware) isAllowedOrigin(origin string) bool {
	if len(m.config.AllowedOrigins) == 0 {
		return true // allow all
	}
	for _, allowed := range m.config.AllowedOrigins {
		if allowed == "*" || allowed == origin {
			return true
		}
	}
	return false
}

func (m *SecurityMiddleware) isAllowedHost(host string) bool {
	// Strip port
	hostname := host
	if idx := strings.LastIndex(host, ":"); idx > 0 {
		hostname = host[:idx]
	}
	// Allow loopback and the bound address
	if isLoopbackAddress(hostname) {
		return true
	}
	return hostname == m.config.BindAddress || hostname == "localhost"
}

func isLoopbackAddress(addr string) bool {
	if addr == "" || addr == "localhost" || addr == "127.0.0.1" || addr == "::1" {
		return true
	}
	ip := net.ParseIP(addr)
	if ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func extractClientIP(r *http.Request, trustedProxies map[string]bool) string {
	// Only trust proxy headers when the direct connection is from a trusted proxy.
	directIP := remoteIP(r)
	if trustedProxies != nil && trustedProxies[directIP] {
		// Check X-Forwarded-For
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			parts := strings.SplitN(xff, ",", 2)
			return strings.TrimSpace(parts[0])
		}
		// Check X-Real-IP
		if xri := r.Header.Get("X-Real-IP"); xri != "" {
			return xri
		}
	}
	// Fall back to RemoteAddr
	return directIP
}

// remoteIP extracts the IP from RemoteAddr, stripping the port.
func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

// ── Rate Limiter ──

type rateLimiter struct {
	mu       sync.Mutex
	limit    int
	counters map[string]*rateCounter
}

type rateCounter struct {
	count    int
	windowAt time.Time
}

func newRateLimiter(limitPerMinute int) *rateLimiter {
	return &rateLimiter{
		limit:    limitPerMinute,
		counters: make(map[string]*rateCounter),
	}
}

func (rl *rateLimiter) Allow(clientIP string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	now := time.Now()
	counter, ok := rl.counters[clientIP]
	if !ok || now.Sub(counter.windowAt) >= time.Minute {
		rl.counters[clientIP] = &rateCounter{count: 1, windowAt: now}
		return true
	}

	counter.count++
	return counter.count <= rl.limit
}
