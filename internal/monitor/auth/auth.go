// Package auth holds the token, Host, Origin and audit rules shared by the
// /monitor API and WebSocket endpoints.
package auth

import (
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"regexp"
	"strings"
)

const (
	// CODE_UNAUTHORIZED marks a missing or wrong token.
	CODE_UNAUTHORIZED = "unauthorized"
	// CODE_FORBIDDEN marks a Host/Origin rejection or a disabled write.
	CODE_FORBIDDEN = "forbidden"
	// CODE_WRITE_DISABLED marks a write in the no-token (read-only) shape.
	CODE_WRITE_DISABLED = "write_disabled"

	// TOKEN_HEADER is the primary token header.
	TOKEN_HEADER = "X-Auth-Token"
	// TOKEN_SCHEME prefixes the Authorization header form.
	TOKEN_SCHEME = "Bearer "
	// WS_PROTOCOL_PREFIX prefixes the WebSocket subprotocol token.
	WS_PROTOCOL_PREFIX = "astrbox.token."
)

// tokenPattern is the frozen http_token charset contract from K3.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)

// ValidTokenCharset reports whether a configured token matches
// ^[A-Za-z0-9_-]{16,128}$; an empty token (no-auth shape) is valid.
func ValidTokenCharset(token string) bool {
	return token == "" || tokenPattern.MatchString(token)
}

// TokenFromHeaders reads X-Auth-Token first, then Authorization: Bearer.
func TokenFromHeaders(r *http.Request) string {
	if provided := r.Header.Get(TOKEN_HEADER); provided != "" {
		return provided
	}
	authorization := r.Header.Get("Authorization")
	if strings.HasPrefix(authorization, TOKEN_SCHEME) {
		return strings.TrimPrefix(authorization, TOKEN_SCHEME)
	}
	return ""
}

// TokenFromProtocols extracts the token from a list of offered WebSocket
// subprotocols and returns the exact protocol string the server must echo.
func TokenFromProtocols(protocols []string) (token, protocol string, ok bool) {
	for _, offered := range protocols {
		if strings.HasPrefix(offered, WS_PROTOCOL_PREFIX) {
			return strings.TrimPrefix(offered, WS_PROTOCOL_PREFIX), offered, true
		}
	}
	return "", "", false
}

// HostAllowed reports whether Host is one of the loopback forms allowed by K3:
// 127.0.0.1[:port], localhost[:port] or [::1][:port].
func HostAllowed(host string) bool {
	hostname := host
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		hostname = parsed
	}
	hostname = strings.TrimSuffix(strings.TrimPrefix(hostname, "["), "]")
	switch strings.ToLower(hostname) {
	case "127.0.0.1", "localhost", "::1":
		return true
	default:
		return false
	}
}

// TokenEqual compares two tokens in constant time. A length mismatch is
// rejected immediately; token length is not secret (the charset contract fixes
// the range), so this leaks nothing beyond what the config already discloses.
func TokenEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1
}

// LocalAddrAllowed reports whether the connection's local address is loopback.
// Unlike HostAllowed it reads the socket's actual local address from the request
// context, which a client cannot spoof with a Host header. A missing context
// key means the request was served in-process (no real network connection) and
// is treated as local.
func LocalAddrAllowed(r *http.Request) bool {
	addr, ok := r.Context().Value(http.LocalAddrContextKey).(net.Addr)
	if !ok || addr == nil {
		return true
	}
	host, _, err := net.SplitHostPort(addr.String())
	if err != nil {
		host = addr.String()
	}
	ip := net.ParseIP(strings.Trim(host, "[]"))
	return ip != nil && ip.IsLoopback()
}

// OriginAllowed reports whether Origin is exactly "http://" + Host; prefix or
// contains matching is forbidden, and Origin: null is rejected.
func OriginAllowed(origin, host string) bool {
	if origin == "" || origin == "null" {
		return false
	}
	return origin == "http://"+host
}

// Middleware requires the configured token on every request; an empty token
// keeps the historical no-auth shape (read-only by way of the callers).
func Middleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if token != "" && !TokenEqual(TokenFromHeaders(r), token) {
			w.Header().Set("WWW-Authenticate", "Bearer")
			WriteError(w, http.StatusUnauthorized, CODE_UNAUTHORIZED, "缺少或错误的 token")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// WriteError answers the frozen {"ok":false,"error":{code,message}} envelope.
// No Access-Control-Allow-Origin header is ever set.
func WriteError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"ok":    false,
		"error": map[string]any{"code": code, "message": message},
	})
}
