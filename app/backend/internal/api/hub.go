package api

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"

	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/auth"
	"github.com/sbrw-evc/umbrella-monitoring/app/backend/internal/model"
)

// Hub fans out live updates (alerts.changed, events) to browsers.
type Hub struct {
	// Origins are host patterns allowed to open the socket besides the
	// same origin (for the dev server), e.g. "localhost:5173".
	Origins []string
	mu      sync.Mutex
	clients map[chan []byte]*auth.Principal
}

// NewHub creates an empty hub.
func NewHub() *Hub { return &Hub{clients: map[chan []byte]*auth.Principal{}} }

// visible reports whether the user may receive the update: the same rules
// as the REST API (permission and business-service scope).
func visible(p *auth.Principal, kind string, v any) bool {
	switch x := v.(type) {
	case model.Alert:
		return p.Can(model.PermIncidentsView) && p.SeesCI(x.CIID)
	case model.Event:
		return p.Can(model.PermEventsView) && p.SeesCI(x.CIID)
	case model.ParseError:
		return p.Can(model.PermEventsView) && p.AllServices
	}
	return kind != "" && p.AllServices
}

// Publish sends {type, data} to every client allowed to see it; slow
// clients drop messages.
func (h *Hub) Publish(kind string, v any) {
	b, err := json.Marshal(map[string]any{"type": kind, "data": v})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	for c, p := range h.clients {
		if !visible(p, kind, v) {
			continue
		}
		select {
		case c <- b:
		default:
		}
	}
}

// serveWS is GET /api/ws behind require: the socket carries the user.
func (s *Server) serveWS(w http.ResponseWriter, r *http.Request) { s.hub.serve(w, r) }

func (h *Hub) serve(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: h.Origins})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ch := make(chan []byte, 256)
	h.mu.Lock()
	h.clients[ch] = auth.From(r.Context())
	h.mu.Unlock()
	defer func() {
		h.mu.Lock()
		delete(h.clients, ch)
		h.mu.Unlock()
	}()
	ctx := conn.CloseRead(r.Context())
	ping := time.NewTicker(25 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-ch:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
