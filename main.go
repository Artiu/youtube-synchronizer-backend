package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"yt-synchronizer/code"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type Room struct {
	receivers []chan []byte
	sync.RWMutex
}

func NewRoom() *Room {
	return &Room{receivers: make([]chan []byte, 0)}
}

func (r *Room) Join(newChan chan []byte) {
	r.Lock()
	r.receivers = append(r.receivers, newChan)
	r.Unlock()
}

func (r *Room) Leave(channel chan []byte) {
	r.Lock()
	for i, receiver := range r.receivers {
		if receiver == channel {
			r.receivers = append(r.receivers[:i], r.receivers[i+1:]...)
			break
		}
	}
	r.Unlock()
}

func (r *Room) Broadcast(message []byte) {
	r.RLock()
	for _, receiver := range r.receivers {
		receiver <- message
	}
	r.RUnlock()
}

func (r *Room) CloseReceivers() {
	r.RLock()
	for _, receiver := range r.receivers {
		close(receiver)
	}
	r.RUnlock()
}

type Server struct {
	codes map[string]*Room
	sync.RWMutex
}

func NewServer() *Server {
	return &Server{codes: make(map[string]*Room)}
}

func main() {
	r := chi.NewRouter()
	s := NewServer()

	r.Use(middleware.Logger)
	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			w.WriteHeader(500)
			return
		}
		go func() {
			var roomCode string
			s.Lock()
			for {
				roomCode = code.GenerateRandom()
				if _, exists := s.codes[roomCode]; !exists {
					break
				}
			}
			room := NewRoom()
			s.codes[roomCode] = room
			s.Unlock()
			encoded, _ := json.Marshal(map[string]string{"code": roomCode})
			wsutil.WriteServerText(conn, encoded)
			defer conn.Close()
			for {
				msg, err := wsutil.ReadClientText(conn)
				if err != nil {
					s.Lock()
					delete(s.codes, roomCode)
					s.Unlock()
					room.CloseReceivers()
					break
				}
				room.Broadcast(msg)
			}
		}()
	})
	r.Get("/room/{roomCode}", func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "roomCode")
		s.Lock()
		room, ok := s.codes[code]
		if !ok {
			s.Unlock()
			w.WriteHeader(404)
			return
		}
		sendChannel := make(chan []byte)
		room.Join(sendChannel)
		s.Unlock()
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		leaveRoom := func() {
			s.Lock()
			room.Leave(sendChannel)
			s.Unlock()
		}
	messageLoop:
		for {
			select {
			case msg, more := <-sendChannel:
				if !more {
					leaveRoom()
					break messageLoop
				}
				fmt.Fprintf(w, "data: %v\n\n", string(msg))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-r.Context().Done():
				leaveRoom()
				break messageLoop
			}
		}

	})

	http.ListenAndServe("127.0.0.1:3000", r)
}
