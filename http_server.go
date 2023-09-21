package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/gobwas/ws"
)

func UnmarshalJSON[T any](data []byte) T {
	var parsed T
	json.Unmarshal(data, &parsed)
	return parsed
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
		hostWs := NewHostWebsocket(conn)

		roomCode, room, err := reconnect.Try(reconnectionKey)
		if err != nil {
			roomCode, room = h.Server.CreateRoom()
			LogCreatedRoom(roomCode)
		} else {
			LogReconnectedToRoom(roomCode)
		}

		logger := GetRoomIPLogger(r.RemoteAddr, roomCode)
		err = hostWs.SendRoomCode(roomCode)
		if err != nil {
			h.Server.RemoveCode(roomCode)
			return
		}

		closed := make(chan bool)
		reconnectionKeyTicker := time.NewTicker(reconnectionTokenSendFreq)

		go func() {
			sendReconnectionKey := func() {
				token := reconnect.GenerateReconnectionToken(roomCode)
				hostWs.SendReconnectKey(token)
			}
			sendReconnectionKey()
			for {
				select {
				case <-reconnectionKeyTicker.C:
					sendReconnectionKey()
				case <-closed:
					return
				}
			}
		}()

		for {
			msg, err := hostWs.ReadMessage()
			if err != nil {
				if errors.Is(err, ErrUndefinedType) {
					continue
				}
				logger.HostDisconnected()
				reconnectionKeyTicker.Stop()
				closed <- true
				reconnectionTimer := time.NewTimer(reconnectionTime)
				room.HostDisconnected()
				select {
				case <-room.reconnected:
					reconnectionTimer.Stop()
					room.HostReconnected()
				case <-reconnectionTimer.C:
					h.Server.RemoveCode(roomCode)
					room.Close()
					logger.RemovingRoom()
				}
				break
			}
			switch m := msg.(type) {
			case SyncMessage:
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdatePath(m.Path)
					v.UpdateIsPaused(m.IsPaused)
					v.UpdateRate(m.Rate)
					v.UpdateTime(m.Time)
				})
			case StartPlayingMessage:
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateIsPaused(false)
					v.UpdateTime(m.Time)
				})
			case PauseMessage:
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateIsPaused(true)
					v.UpdateTime(m.Time)
				})
			case PathChangeMessage:
				room.UpdateVideoState(func(v *VideoState) {
					room.videoState.UpdatePath(m.Path)
				})
			case RateChangeMessage:
				room.UpdateVideoState(func(v *VideoState) {
					v.UpdateRate(m.Rate)
				})
			case RemoveRoomMessage:
				reconnectionKeyTicker.Stop()
				closed <- true
				h.Server.RemoveCode(roomCode)
				room.Close()
				logger.RemovingRoom()
				return
			}
			room.HostMessage(msg)
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
		receiverSSE := NewReceiverSSE(w, flusher)
		videoState := room.GetPredictedVideoState()
		receiverSSE.SendSyncMessage(videoState)
		if !room.IsHostConnected() {
			receiverSSE.SendHostDisconnectedMessage()
		}
		logger := GetRoomIPLogger(r.RemoteAddr, code)
		logger.JoinedRoom()
		sendChannel := make(chan []byte)
		room.Join(sendChannel)
	messageLoop:
		for {
			select {
			case msg, more := <-sendChannel:
				if !more {
					receiverSSE.SendRoomClosedMessage()
					logger.LeftRoom()
					break messageLoop
				}
				receiverSSE.SendByteSlice(msg)
			case <-r.Context().Done():
				room.Leave(sendChannel)
				logger.LeftRoom()
				break messageLoop
			}
		}

	}
}
