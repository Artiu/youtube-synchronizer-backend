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
)

func UnmarshalJSON[T any](data []byte) T {
	var parsed T
	json.Unmarshal(data, &parsed)
	return parsed
}

func MustMarshalJSON(data any) []byte {
	encoded, _ := json.Marshal(data)
	return encoded
}

type HTTPHandler struct {
	Server *Server
}

func NewHTTPServer(server *Server, reconnect *Reconnect) http.Handler {
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

	r.Get("/ws", httpHandler.websocket(reconnect))
	r.Get("/room/{roomCode}/path", httpHandler.getRoomCurrentPath())
	r.Get("/room/{roomCode}", httpHandler.getRoomEventStream())
	return r
}

func (h HTTPHandler) websocket(reconnect *Reconnect) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		reconnectionKey := r.URL.Query().Get("reconnectKey")
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			LogErrorWhileUpgradingHTTP(err)
			return
		}
		defer conn.Close()

		roomCode, room, err := reconnect.Try(reconnectionKey)
		if err != nil {
			roomCode, room = h.Server.CreateRoom()
			LogCreatedRoom(roomCode)
		} else {
			LogReconnectedToRoom(roomCode)
		}

		logger := GetRoomIPLogger(r.RemoteAddr, roomCode)
		encoded := MustMarshalJSON(map[string]string{"type": "code", "code": roomCode})
		err = wsutil.WriteServerText(conn, encoded)
		if err != nil {
			h.Server.RemoveCode(roomCode)
			return
		}

		closed := make(chan bool)
		ticker := time.NewTicker(reconnectionTokenSendFreq)

		go func() {
			sendReconnectionKey := func() {
				token, err := reconnect.GenerateReconnectionToken(roomCode)
				if err != nil {
					return
				}
				encoded := MustMarshalJSON(map[string]string{"type": "reconnectKey", "key": token})
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
				logger.HostDisconnected()
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
					logger.RemovingRoom()
				}
				break
			}
			message := UnmarshalJSON[struct {
				Type string `json:"type"`
			}](msg)
			switch message.Type {
			case "sync":
				parsedMessage := UnmarshalJSON[struct {
					Path     string  `json:"path"`
					Time     float64 `json:"time"`
					Rate     float32 `json:"rate"`
					IsPaused bool    `json:"isPaused"`
				}](msg)
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdatePath(parsedMessage.Path)
					v.UpdateIsPaused(parsedMessage.IsPaused)
					v.UpdateRate(parsedMessage.Rate)
					v.UpdateTime(parsedMessage.Time)
				})
			case "startPlaying":
				parsedMessage := UnmarshalJSON[struct {
					Time float64 `json:"time"`
				}](msg)
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateIsPaused(false)
					v.UpdateTime(parsedMessage.Time)
				})
			case "pause":
				parsedMessage := UnmarshalJSON[struct {
					Time float64 `json:"time"`
				}](msg)
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateIsPaused(true)
					v.UpdateTime(parsedMessage.Time)
				})
			case "pathChange":
				parsedMessage := UnmarshalJSON[struct {
					Path string `json:"path"`
				}](msg)
				room.UpdateVideoState(func(v *VideoState) {
					room.videoState.UpdatePath(parsedMessage.Path)
				})
			case "rateChange":
				parsedMessage := UnmarshalJSON[struct {
					Rate float32 `json:"rate"`
				}](msg)
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateRate(parsedMessage.Rate)
				})
			case "removeRoom":
				ticker.Stop()
				closed <- true
				h.Server.RemoveCode(roomCode)
				room.CloseReceivers()
				logger.RemovingRoom()
				return
			}
			room.Broadcast(msg)
		}
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
			disconnectMsg := MustMarshalJSON(map[string]string{"type": "hostDisconnected"})
			fmt.Fprintf(w, "data: %v\n\n", string(disconnectMsg))
		}
		flusher.Flush()
		logger := GetRoomIPLogger(r.RemoteAddr, code)
		logger.JoinedRoom()
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
					logger.LeftRoom()
					break messageLoop
				}
				fmt.Fprintf(w, "data: %v\n\n", string(msg))
				flusher.Flush()
			case <-r.Context().Done():
				room.Leave(sendChannel)
				logger.LeftRoom()
				break messageLoop
			}
		}

	}
}
