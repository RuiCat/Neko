// WebSocket handshake for /monitor/ws: Origin, subprotocol token and topic
// parsing, dispatched before the REST token middleware.
package ws

import (
	"net/http"
	"strings"

	"github.com/RuiCat/Neko/internal/monitor/auth"
	"github.com/coder/websocket"
)

// WS_PATH is the frozen WebSocket endpoint.
const WS_PATH = "/monitor/ws"

// Serve performs the handshake and runs one connection until it closes.
func (h *Hub) Serve(w http.ResponseWriter, r *http.Request, token string) {
	origin := r.Header.Get("Origin")
	if origin != "" && !auth.OriginAllowed(origin, r.Host) {
		auth.WriteError(w, http.StatusForbidden, auth.CODE_FORBIDDEN, "WS Origin 不合法")
		return
	}
	protocol := ""
	if token != "" {
		offeredToken, offeredProtocol, ok := auth.TokenFromProtocols(offeredProtocols(r))
		if !ok || !auth.TokenEqual(offeredToken, token) {
			auth.WriteError(w, http.StatusUnauthorized, auth.CODE_UNAUTHORIZED, "WS 缺少或错误的 token 子协议")
			return
		}
		protocol = offeredProtocol
	}
	options := &websocket.AcceptOptions{}
	if protocol != "" {
		options.Subprotocols = []string{protocol}
	}
	conn, err := websocket.Accept(w, r, options)
	if err != nil {
		return
	}
	query := r.URL.Query()
	topics := parseTopics(strings.Join(query["topics"], ","))
	client := newClient(h, conn, topics, query.Get("entity"), splitList(query["paths"]...))
	client.Run(r.Context())
}

// offeredProtocols flattens the Sec-WebSocket-Protocol request header.
func offeredProtocols(r *http.Request) []string {
	protocols := make([]string, 0)
	for _, value := range r.Header.Values("Sec-WebSocket-Protocol") {
		for _, part := range strings.Split(value, ",") {
			part = strings.TrimSpace(part)
			if part != "" {
				protocols = append(protocols, part)
			}
		}
	}
	return protocols
}
