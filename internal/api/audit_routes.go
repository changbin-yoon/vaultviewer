package api

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/accesslens/accesslens/internal/audit"
	"github.com/accesslens/accesslens/internal/auth"
	"github.com/accesslens/accesslens/internal/model"
)

const (
	wsPongWait   = 60 * time.Second
	wsPingPeriod = (wsPongWait * 9) / 10
	wsWriteWait  = 10 * time.Second
)

func registerAuditRoutes(mux *http.ServeMux, d Deps) {
	mux.HandleFunc("GET /api/audit", auth.RequireAdmin(d.Sessions, func(w http.ResponseWriter, r *http.Request, _ model.User) {
		entries, err := d.Recorder.All()
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(entries)
	}))

	mux.HandleFunc("GET /ws/audit", auditWebSocketHandler(d.Sessions, d.Recorder, d.CORSOrigin))
}

// auditWebSocketHandler streams every new audit log entry to the client as
// audit.MemoryRecorder.Record is called, so the audit log view updates
// live instead of polling. Auth uses a ?token= query parameter rather than
// the Authorization header every other endpoint requires, since browsers
// cannot set custom headers on a WebSocket handshake request.
func auditWebSocketHandler(sm *auth.SessionManager, recorder *audit.MemoryRecorder, corsOrigin string) http.HandlerFunc {
	upgrader := websocket.Upgrader{CheckOrigin: wsCheckOrigin(corsOrigin)}

	return func(w http.ResponseWriter, r *http.Request) {
		user, err := sm.Verify(r.URL.Query().Get("token"))
		if err != nil {
			http.Error(w, "invalid or expired session", http.StatusUnauthorized)
			return
		}
		if !user.Role.IsAdmin() {
			http.Error(w, "role does not permit admin access", http.StatusForbidden)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Printf("audit ws: upgrade failed: %v", err)
			return
		}
		defer conn.Close()

		ch, unsubscribe := recorder.Subscribe()
		defer unsubscribe()

		conn.SetReadDeadline(time.Now().Add(wsPongWait))
		conn.SetPongHandler(func(string) error {
			conn.SetReadDeadline(time.Now().Add(wsPongWait))
			return nil
		})

		// The client never sends messages; this goroutine's only job is to
		// notice the connection closing (tab closed, network drop) via a
		// read error, so the main loop below can stop promptly.
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			for {
				if _, _, err := conn.NextReader(); err != nil {
					return
				}
			}
		}()

		ticker := time.NewTicker(wsPingPeriod)
		defer ticker.Stop()

		for {
			select {
			case <-closed:
				return
			case entry, ok := <-ch:
				if !ok {
					return
				}
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteJSON(entry); err != nil {
					return
				}
			case <-ticker.C:
				conn.SetWriteDeadline(time.Now().Add(wsWriteWait))
				if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
					return
				}
			}
		}
	}
}
