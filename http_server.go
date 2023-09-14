package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/rs/zerolog/log"
)

type HTTPHandler struct {
	Server *Server
}

func NewHTTPServer(server *Server, reconnectJWT *ReconnectJWT) http.Handler {
	httpHandler := HTTPHandler{server}
	r := chi.NewRouter()
	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"chrome-extension://*", "https://www.youtube.com"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: false,
	}))
	r.Use(middleware.RealIP)
	r.Use(httprate.Limit(30, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP, httprate.KeyByEndpoint)))
	r.Use(middleware.Heartbeat("/"))

	r.Get("/ws", httpHandler.websocket(reconnectJWT))
	r.Get("/room/{roomCode}/path", httpHandler.getRoomCurrentPath())
	r.Get("/room/{roomCode}", httpHandler.getRoomEventStream())
	return r
}

func (h HTTPHandler) websocket(reconnectJWT *ReconnectJWT) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var roomCode string
		var room *Room
		reconnectionKey := r.URL.Query().Get("reconnectKey")
		if reconnectionKey != "" {
			roomCode = reconnectJWT.GetRoomCode(reconnectionKey)
		}
		if roomCode != "" {
			room, _ = h.Server.GetRoom(roomCode)
			if room != nil {
				if room.IsHostConnected() {
					room = nil
				} else {
					room.SendHostReconnected()
					log.Info().Str("room-code", roomCode).Msg("Reconnected")
				}
			} else {
				room = NewRoom()
				h.Server.RegisterCode(roomCode, room)
				log.Info().Str("room-code", roomCode).Msg("Reconnected")
			}
		}
		if room == nil {
			for {
				roomCode = GenerateRandomCode()
				if _, exists := h.Server.GetRoom(roomCode); !exists {
					break
				}
			}
			log.Info().Str("room-code", roomCode).Msg("Created room")
			room = NewRoom()
			h.Server.RegisterCode(roomCode, room)
		}

		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Error().Str("msg", "Error while upgrading HTTP").Err(err).Msg("")
			return
		}
		go func() {
			logger := GetLogger(r.RemoteAddr, roomCode)
			encoded, _ := json.Marshal(map[string]string{"type": "code", "code": roomCode})
			defer conn.Close()
			err = wsutil.WriteServerText(conn, encoded)
			if err != nil {
				logger.Info().Msg("Removing room")
				h.Server.RemoveCode(roomCode)
				return
			}

			closed := make(chan bool)
			ticker := time.NewTicker(time.Minute)

			go func() {
				sendReconnectionKey := func() {
					token, err := reconnectJWT.Generate(roomCode)
					if err != nil {
						logger.Err(err).Send()
						return
					}
					encoded, _ = json.Marshal(map[string]string{"type": "reconnectKey", "key": token})
					wsutil.WriteServerText(conn, encoded)
				}
				sendReconnectionKey()
				for {
					select {
					case <-ticker.C:
						sendReconnectionKey()
					case <-closed:
						return
					}
				}
			}()

			for {
				msg, err := wsutil.ReadClientText(conn)
				if err != nil {
					logger.Info().Msg("Disconnected")
					ticker.Stop()
					closed <- true
					reconnectionTimer := time.NewTimer(reconnectionTime)
					room.UpdateReconnectChannel(make(chan bool))
					disconnectMsg, _ := json.Marshal(map[string]string{"type": "hostDisconnected"})
					room.Broadcast(disconnectMsg)
					select {
					case <-room.reconnected:
						reconnectionTimer.Stop()
						room.UpdateReconnectChannel(nil)
						reconnectedMsg, _ := json.Marshal(map[string]string{"type": "hostReconnected"})
						room.Broadcast(reconnectedMsg)
					case <-reconnectionTimer.C:
						h.Server.RemoveCode(roomCode)
						room.CloseReceivers()
						logger.Info().Msg("Removing room")
					}
					break
				}
				var message struct {
					Type string `json:"type"`
				}
				json.Unmarshal(msg, &message)
				switch message.Type {
				case "sync":
					var parsedMessage struct {
						Path     string  `json:"path"`
						Time     float64 `json:"time"`
						Rate     float32 `json:"rate"`
						IsPaused bool    `json:"isPaused"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.UpdateVideoState(func(v *VideoState) {
						v.UpdatePath(parsedMessage.Path)
						v.UpdateIsPaused(parsedMessage.IsPaused)
						v.UpdateRate(parsedMessage.Rate)
						v.UpdateTime(parsedMessage.Time)
					})
				case "startPlaying":
					var parsedMessage struct {
						Time float64 `json:"time"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.UpdateVideoState(func(v *VideoState) {
						v.UpdateIsPaused(false)
						v.UpdateTime(parsedMessage.Time)
					})
				case "pause":
					var parsedMessage struct {
						Time float64 `json:"time"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.UpdateVideoState(func(v *VideoState) {
						v.UpdateIsPaused(true)
						v.UpdateTime(parsedMessage.Time)
					})
				case "pathChange":
					var parsedMessage struct {
						Path string `json:"path"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.UpdateVideoState(func(v *VideoState) {
						room.videoState.UpdatePath(parsedMessage.Path)
					})
				case "rateChange":
					var parsedMessage struct {
						Rate float32 `json:"rate"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.UpdateVideoState(func(v *VideoState) {
						v.UpdateRate(parsedMessage.Rate)
					})
				case "removeRoom":
					ticker.Stop()
					closed <- true
					h.Server.RemoveCode(roomCode)
					room.CloseReceivers()
					logger.Info().Msg("Removing room")
					return
				}
				room.Broadcast(msg)
			}
		}()
	}
}

func (h HTTPHandler) getRoomCurrentPath() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "roomCode")
		room, exists := h.Server.GetRoom(code)
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		path := room.GetVideoPath()
		w.Write([]byte(path))
	}
}

func (h HTTPHandler) getRoomEventStream() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "HTTP Streaming not supported!", http.StatusBadRequest)
			return
		}
		code := chi.URLParam(r, "roomCode")
		room, exists := h.Server.GetRoom(code)
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		videoState := room.GetPredictedVideoState()
		initialData, _ := json.Marshal(struct {
			Type string `json:"type"`
			VideoState
		}{Type: "sync", VideoState: videoState})
		fmt.Fprintf(w, "data: %v\n\n", string(initialData))
		if !room.IsHostConnected() {
			disconnectMsg, _ := json.Marshal(map[string]string{"type": "hostDisconnected"})
			fmt.Fprintf(w, "data: %v\n\n", string(disconnectMsg))
		}
		flusher.Flush()
		logger := GetLogger(r.RemoteAddr, code)
		logger.Info().Msg("Joined room")
		sendChannel := make(chan []byte)
		room.Join(sendChannel)
	messageLoop:
		for {
			select {
			case msg, more := <-sendChannel:
				if !more {
					room.Leave(sendChannel)
					msg, _ = json.Marshal(map[string]string{"type": "close"})
					fmt.Fprintf(w, "data: %v\n\n", string(msg))
					flusher.Flush()
					logger.Info().Msg("Left room")
					break messageLoop
				}
				fmt.Fprintf(w, "data: %v\n\n", string(msg))
				flusher.Flush()
			case <-r.Context().Done():
				room.Leave(sendChannel)
				logger.Info().Msg("Left room")
				break messageLoop
			}
		}

	}
}
