// HTTP transport guards: token auth and loopback detection.
package main

import (
	"net"
	"net/http"
	"strings"
)

// tokenMiddleware requires "Authorization: Bearer <token>" or "X-Auth-Token".
func tokenMiddleware(next http.Handler, token string) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		provided := request.Header.Get("X-Auth-Token")
		if provided == "" {
			authorization := request.Header.Get("Authorization")
			if strings.HasPrefix(authorization, "Bearer ") {
				provided = strings.TrimPrefix(authorization, "Bearer ")
			}
		}
		if provided != token {
			writer.Header().Set("WWW-Authenticate", "Bearer")
			http.Error(writer, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(writer, request)
	})
}

// isLoopbackAddr reports whether the listen address is loopback-only.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		host = addr
	}
	if host == "" {
		return false
	}
	if host == "localhost" {
		return true
	}
	parsed := net.ParseIP(host)
	return parsed != nil && parsed.IsLoopback()
}
