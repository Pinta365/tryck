package devserver

import (
	"context"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/coder/websocket"
)

// message is sent to browsers on /__tryck/ws.
type message struct {
	Type        string           `json:"type"` // "reload" or "error"
	Diagnostics []diagnosticJSON `json:"diagnostics,omitempty"`
}

type diagnosticJSON struct {
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
	Col      int    `json:"col,omitempty"`
	Severity string `json:"severity"`
	Code     string `json:"code"`
	Message  string `json:"message"`
	Text     string `json:"text"` // formatted file:line:col: severity[code]: message
}

// hub tracks WebSocket clients and broadcasts messages to them. A new
// client immediately receives the last error, if the site is broken.
type hub struct {
	mu      sync.Mutex
	clients map[chan []byte]struct{}
	pending []byte // last error message, nil after a good build
}

func newHub() *hub { return &hub{clients: map[chan []byte]struct{}{}} }

func (h *hub) broadcast(m message) {
	b, err := json.Marshal(m)
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	if m.Type == "error" {
		h.pending = b
	} else {
		h.pending = nil
	}
	for c := range h.clients {
		select {
		case c <- b:
		default: // a slow client misses this message; the next one still arrives
		}
	}
}

func (h *hub) subscribe() chan []byte {
	c := make(chan []byte, 8)
	h.mu.Lock()
	h.clients[c] = struct{}{}
	if h.pending != nil {
		c <- h.pending
	}
	h.mu.Unlock()
	return c
}

func (h *hub) unsubscribe(c chan []byte) {
	h.mu.Lock()
	delete(h.clients, c)
	h.mu.Unlock()
}

func (h *hub) count() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// serveWS upgrades the request and forwards hub messages until the
// client goes away or ctx ends.
func (h *hub) serveWS(ctx context.Context, w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, nil) // same-origin only
	if err != nil {
		return
	}
	defer conn.CloseNow()
	c := h.subscribe()
	defer h.unsubscribe(c)

	// Reading is required to notice closes; clients send nothing.
	readCtx := conn.CloseRead(ctx)
	ping := time.NewTicker(30 * time.Second)
	defer ping.Stop()
	for {
		select {
		case <-readCtx.Done():
			return
		case b := <-c:
			wctx, cancel := context.WithTimeout(readCtx, 5*time.Second)
			err := conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ping.C:
			pctx, cancel := context.WithTimeout(readCtx, 5*time.Second)
			err := conn.Ping(pctx)
			cancel()
			if err != nil {
				return
			}
		}
	}
}
