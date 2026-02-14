package ws

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"time"

	"nhooyr.io/websocket"
)

// WSMessage represents a message from client to server over WebSocket.
type WSMessage struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// SendMessageRequest is the data for a "send_message" WebSocket message.
type SendMessageRequest struct {
	To   string `json:"to"`
	Body string `json:"body"`
}

// MessageHandler is called when a client sends a message via WebSocket.
type MessageHandler func(to, body string) error

// SessionLookup retrieves a user ID from a token. Returns ("", false) if invalid.
type SessionLookup func(token string) (userID string, ok bool)

// HandleWS upgrades an HTTP connection to WebSocket and manages the client lifecycle.
func HandleWS(hub *Hub, msgHandler MessageHandler, sessionLookup SessionLookup) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Authenticate via token query parameter
		var userID string
		if sessionLookup != nil {
			token := r.URL.Query().Get("token")
			if token == "" {
				http.Error(w, "token required", http.StatusUnauthorized)
				return
			}
			uid, ok := sessionLookup(token)
			if !ok {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}
			userID = uid
		}

		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
			InsecureSkipVerify: true, // Allow any origin for dev
		})
		if err != nil {
			log.Printf("[ws] accept error: %v", err)
			return
		}

		client := &Client{
			ID:     r.RemoteAddr,
			UserID: userID,
			Send:   make(chan []byte, 64),
		}

		hub.Register(client)
		defer func() {
			hub.Unregister(client)
			conn.Close(websocket.StatusNormalClosure, "")
		}()

		ctx, cancel := context.WithCancel(r.Context())
		defer cancel()

		// Write pump: sends messages from hub to client
		go func() {
			defer cancel()
			for {
				select {
				case <-ctx.Done():
					return
				case msg, ok := <-client.Send:
					if !ok {
						return
					}
					err := conn.Write(ctx, websocket.MessageText, msg)
					if err != nil {
						return
					}
				}
			}
		}()

		// Read pump: reads messages from client
		for {
			_, data, err := conn.Read(ctx)
			if err != nil {
				return
			}

			var wsMsg WSMessage
			if err := json.Unmarshal(data, &wsMsg); err != nil {
				continue
			}

			switch wsMsg.Type {
			case "send_message":
				var req SendMessageRequest
				if err := json.Unmarshal(wsMsg.Data, &req); err != nil {
					continue
				}
				if msgHandler != nil && req.To != "" && req.Body != "" {
					if err := msgHandler(req.To, req.Body); err != nil {
						log.Printf("[ws] send message error: %v", err)
					}
				}
			case "ping":
				resp, _ := json.Marshal(map[string]string{"type": "pong", "time": time.Now().UTC().Format(time.RFC3339)})
				select {
				case client.Send <- resp:
				default:
				}
			}
		}
	}
}
