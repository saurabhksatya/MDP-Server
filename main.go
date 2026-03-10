package main

import (
	"errors"
	"fmt"
	"log"
	"mdpserver/classes"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true // Adjust for production
	},
}

const cacheTTL = 5 * time.Minute

type Hub struct {
	devices map[string]*classes.Device
	clients map[string]*classes.Client
	mu      sync.RWMutex
}

type CacheEntry struct {
	Value     string
	ExpiresAt time.Time
}

type AuthCache struct {
	data map[string]CacheEntry
	mu   sync.RWMutex
}

func NewAuthCache() *AuthCache {
	return &AuthCache{
		data: make(map[string]CacheEntry),
	}
}

func (c *AuthCache) Get(key string) (string, bool) {
	c.mu.RLock()
	entry, ok := c.data[key]
	c.mu.RUnlock()

	if !ok {
		return "", false
	}

	if time.Now().After(entry.ExpiresAt) {
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		return "", false
	}

	return entry.Value, true
}

func (c *AuthCache) Set(key string, value string) {
	c.mu.Lock()
	c.data[key] = CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(cacheTTL),
	}
	c.mu.Unlock()
}

func NewHub() *Hub {
	return &Hub{
		devices: make(map[string]*classes.Device),
		clients: make(map[string]*classes.Client),
	}
}

func (h *Hub) AddDevice(id string, clientId string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.devices[id] = &classes.Device{DeviceID: id, Conn: conn, ClientID: clientId}
	log.Println("Device connected:", id)
}

func (h *Hub) AddClient(id string, conn *websocket.Conn) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.clients[id] = &classes.Client{ClientId: id, Conn: conn}
	log.Println("Client connected:", id)
}

func (h *Hub) RemoveDevice(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	device, ok := h.devices[id]
	if !ok {
		return
	}
	device.Conn.Close()
	delete(h.devices, id)
	log.Println("Device disconnected:", id)
}

func (h *Hub) RemoveClient(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.clients, id)
	log.Println("Client disconnected:", id)
}

func (h *Hub) SendToDevice(deviceID string, message []byte) error {
	h.mu.RLock()
	device, ok := h.devices[deviceID]
	h.mu.RUnlock()

	if !ok {
		return errors.New("Device not found")
	}

	return device.Conn.WriteMessage(websocket.TextMessage, message)
}

func (h *Hub) SendToDeviceFromClient(deviceID string, clientID string, message []byte) error {
	device, ok := h.devices[deviceID]

	if !ok {
		return errors.New("Device not found")
	}

	if device.ClientID != clientID {
		return errors.New("Client ID mismatch")
	}

	return h.SendToDevice(deviceID, message)
}

type Response struct {
	Message string `json:"message"`
	Success bool   `json:"success"`
}

func main() {
	hub := NewHub()
	r := mux.NewRouter()

	clientCache := NewAuthCache()

	r.HandleFunc("/devices/{device_id}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		deviceID := vars["device_id"]
		key := vars["key"]

		authenticated, clientId := classes.AuthenticateDevice(deviceID, key)
		if !authenticated {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		hub.AddDevice(deviceID, clientId, conn)

		defer func() {
			hub.RemoveDevice(deviceID)
			conn.Close()
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}
			fmt.Println(string(msg))

		}
	})

	r.HandleFunc("/client/{client_id}/{key}", func(w http.ResponseWriter, r *http.Request) {
		vars := mux.Vars(r)
		clientID := vars["client_id"]
		key := vars["key"]

		_, ok := hub.clients[clientID]
		if ok {
			http.Error(w, http.StatusText(http.StatusUnauthorized), http.StatusUnauthorized)
			return
		}

		cacheClient, ok := clientCache.Get(clientID)
		if !ok {
			if !classes.AuthenticateClient(clientID, key) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			clientCache.Set(clientID, key)
		} else {
			if cacheClient != key {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
		}

		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Println(err)
			return
		}

		hub.AddClient(clientID, conn)

		defer func() {
			hub.RemoveClient(clientID)
			conn.Close()
		}()

		for {
			_, msg, err := conn.ReadMessage()
			if err != nil {
				break
			}

			_, ok := hub.clients[clientID]
			if !ok {
				break
			}

			message := string(msg)

			online_ids := map[string]bool{}

			if message == "ONLINE" {
				for _, device := range hub.devices {
					if device.ClientID == clientID {
						online_ids[device.DeviceID] = true
					}
				}

				// return ids of all online devices

				conn.WriteJSON(online_ids)
				continue
			}

			// Find first colon
			deviceIdEnd := -1
			for i := 0; i < len(message); i++ {
				if message[i] == ':' {
					deviceIdEnd = i
					break
				}
			}

			if deviceIdEnd == -1 {
				conn.WriteJSON(Response{Message: message, Success: false})
				continue
			}

			deviceId := message[:deviceIdEnd]
			data := message[deviceIdEnd+1:]

			err = hub.SendToDeviceFromClient(deviceId, clientID, []byte(data))

			if err != nil {
				conn.WriteJSON(Response{Message: message, Success: false})
			} else {
				conn.WriteJSON(Response{Message: message, Success: true})
			}
		}
	})

	log.Println("Server running on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
