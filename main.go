package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"yt-synchronizer/code"

	"github.com/go-chi/chi/v5"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
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

func GetLogger(ip string, roomCode string) zerolog.Logger {
	return log.With().Str("ip", ip).Str("room-code", roomCode).Logger()
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix

	r := chi.NewRouter()
	s := NewServer()

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
			logger := GetLogger(r.RemoteAddr, roomCode)
			logger.Info().Msg("Created room")
			encoded, _ := json.Marshal(map[string]string{"code": roomCode})
			wsutil.WriteServerText(conn, encoded)
			defer conn.Close()
			for {
				msg, err := wsutil.ReadClientText(conn)
				if err != nil {
					logger.Info().Msg("Removing room")
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
		s.RLock()
		room, ok := s.codes[code]
		s.RUnlock()
		if !ok {
			w.WriteHeader(404)
			return
		}
		sendChannel := make(chan []byte)
		room.Join(sendChannel)
		logger := GetLogger(r.RemoteAddr, code)
		logger.Info().Msg("Joined room")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
	messageLoop:
		for {
			select {
			case msg, more := <-sendChannel:
				if !more {
					room.Leave(sendChannel)
					logger.Info().Msg("Left room")
					break messageLoop
				}
				fmt.Fprintf(w, "data: %v\n\n", string(msg))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-r.Context().Done():
				room.Leave(sendChannel)
				logger.Info().Msg("Left room")
				break messageLoop
			}
		}

	})

	log.Info().Msg("Starting server on port 3000")
	http.ListenAndServe("127.0.0.1:3000", r)
}
