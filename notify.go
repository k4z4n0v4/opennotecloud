package main

import (
	"sync"
)

// Client registry for Socket.IO push notifications.
// This is multi-device infrastructure: when a file changes on one device,
// the server can push a ServerMessage to other connected devices to trigger
// auto-sync. Currently unused because push notifications are not implemented
// (requires multiple devices to test). The tablet's AutoSyncManager listens
// for ServerMessage events with operation types like DOWNLOADFILE, STARTSYNC, etc.
type wsClient struct {
	userID     int64
	deviceType string
	send       chan string
	done       chan struct{}
}

type notifyManager struct {
	mu      sync.RWMutex
	clients map[int64][]*wsClient // userID → clients
}

var notifier = &notifyManager{
	clients: make(map[int64][]*wsClient),
}

func (n *notifyManager) register(client *wsClient) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.clients[client.userID] = append(n.clients[client.userID], client)
}

func (n *notifyManager) unregister(client *wsClient) {
	n.mu.Lock()
	defer n.mu.Unlock()
	clients := n.clients[client.userID]
	for i, c := range clients {
		if c == client {
			n.clients[client.userID] = append(clients[:i], clients[i+1:]...)
			break
		}
	}
	if len(n.clients[client.userID]) == 0 {
		delete(n.clients, client.userID)
	}
}
