package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"

	"golang.org/x/net/websocket"
)

func handleSocketIO(cfg *Config) http.HandlerFunc {
	wsServer := &websocket.Server{
		Handshake: func(config *websocket.Config, r *http.Request) error {
			config.Origin = nil
			return nil
		},
		Handler: func(ws *websocket.Conn) {
			defer ws.Close()
			remote := ws.Request().RemoteAddr
			q := ws.Request().URL.Query()
			token, deviceType := q.Get("token"), q.Get("type")

			log.Printf("WS [%s] CONNECT device=%q", remote, deviceType)
			userID, _, err := validateJWTToken(cfg, token)
			if err != nil {
				log.Printf("WS [%s] AUTH FAILED: %v", remote, err)
				sendSocketIOEvent(ws, "ServerMessage", map[string]interface{}{"code": "403", "msg": "token error"})
				return
			}
			log.Printf("WS [%s] AUTH OK user=%d device=%s", remote, userID, deviceType)

			sid := generateNonce()
			openPayload, _ := json.Marshal(map[string]interface{}{
				"sid": sid, "upgrades": []string{}, "pingInterval": 25000, "pingTimeout": 5000,
			})
			openMsg := "0" + string(openPayload)
			log.Printf("WS [%s] SEND: %s", remote, openMsg)
			websocket.Message.Send(ws, openMsg)
			log.Printf("WS [%s] SEND: 40", remote)
			websocket.Message.Send(ws, "40")

			client := &wsClient{userID: userID, deviceType: deviceType, send: make(chan string, 16), done: make(chan struct{})}
			notifier.register(client)
			defer func() {
				log.Printf("WS [%s] DISCONNECT user=%d device=%s", remote, userID, deviceType)
				notifier.unregister(client)
				close(client.done)
			}()

			go func() {
				for {
					select {
					case msg, ok := <-client.send:
						if !ok {
							return
						}
						websocket.Message.Send(ws, msg)
					case <-client.done:
						return
					}
				}
			}()

			for {
				var msg string
				if err := websocket.Message.Receive(ws, &msg); err != nil {
					return
				}
				if msg == "2" {
					websocket.Message.Send(ws, "3")
					continue
				}
				log.Printf("WS [%s] RECV: %s", remote, msg)
				if !strings.HasPrefix(msg, "42") {
					continue
				}
				var arr []json.RawMessage
				if json.Unmarshal([]byte(msg[2:]), &arr) != nil || len(arr) < 1 {
					continue
				}
				var eventName string
				json.Unmarshal(arr[0], &eventName)

				switch eventName {
				case "ratta_ping":
					if _, _, err := validateJWTToken(cfg, token); err != nil {
						sendSocketIOEvent(ws, "ServerMessage", map[string]interface{}{"code": "403", "msg": "token error"})
						return
					}
					log.Printf("WS [%s] SEND: 42[\"ratta_ping\",\"Received\"]", remote)
					sendSocketIOEvent(ws, "ratta_ping", "Received")
				case "ClientMessage":
					if len(arr) >= 2 {
						var data string
						json.Unmarshal(arr[1], &data)
						if data == "status" {
							if _, _, err := validateJWTToken(cfg, token); err != nil {
								return
							}
							sendSocketIOEvent(ws, "ServerMessage", "true")
						}
					}
				}
			}
		},
	}
	return func(w http.ResponseWriter, r *http.Request) {
		log.Printf("=> WS UPGRADE %s %s upgrade=%q connection=%q", r.URL.Path, r.RemoteAddr, r.Header.Get("Upgrade"), r.Header.Get("Connection"))
		wsServer.ServeHTTP(w, r)
	}
}

func sendSocketIOEvent(ws *websocket.Conn, event string, data interface{}) {
	eventJSON, _ := json.Marshal(event)
	dataJSON, _ := json.Marshal(data)
	websocket.Message.Send(ws, fmt.Sprintf("42[%s,%s]", string(eventJSON), string(dataJSON)))
}
