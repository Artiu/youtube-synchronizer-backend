package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"
	"yt-synchronizer/code"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"github.com/go-chi/httprate"
	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
	"github.com/golang-jwt/jwt/v4"
	"github.com/joho/godotenv"
	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

var jwtSecret string

const reconnectionTime = time.Minute * 2

func GetLogger(ip string, roomCode string) zerolog.Logger {
	return log.With().Str("ip", ip).Str("room-code", roomCode).Logger()
}

func GenerateReconnectionJWT(roomCode string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"roomCode": roomCode, "exp": jwt.NewNumericDate(time.Now().Add(reconnectionTime))})
	return token.SignedString([]byte(jwtSecret))
}

func GetRoomCodeFromReconnectionJWT(tokenString string) string {
	token, _ := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(jwtSecret), nil
	})
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims["roomCode"].(string)
	}
	return ""
}

func main() {
	zerolog.TimeFieldFormat = zerolog.TimeFormatUnix
	godotenv.Load()
	port := os.Getenv("PORT")
	if port == "" {
		port = "3000"
	}
	jwtSecret = os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		panic("JWT_SECRET is not provided!")
	}

	r := chi.NewRouter()
	s := NewServer()

	r.Use(middleware.Recoverer)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"chrome-extension://*", "https://www.youtube.com"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: false,
	}))
	r.Use(middleware.RealIP)
	r.Use(httprate.Limit(30, time.Minute, httprate.WithKeyFuncs(httprate.KeyByIP, httprate.KeyByEndpoint)))
	r.Use(middleware.Heartbeat("/"))

	r.Get("/ws", func(w http.ResponseWriter, r *http.Request) {
		var roomCode string
		var room *Room
		reconnectionKey := r.URL.Query().Get("reconnectKey")
		if reconnectionKey != "" {
			roomCode = GetRoomCodeFromReconnectionJWT(reconnectionKey)
		}
		if roomCode != "" {
			room, _ = s.GetRoom(roomCode)
			if room != nil {
				room.RLock()
				if room.reconnected == nil {
					room.RUnlock()
					room = nil
				} else {
					room.reconnected <- true
					room.RUnlock()
					log.Info().Str("room-code", roomCode).Msg("Reconnected")
				}
			} else {
				room = NewRoom()
				s.RegisterCode(roomCode, room)
			}
		}
		if room == nil {
			for {
				roomCode = code.GenerateRandom()
				if _, exists := s.GetRoom(roomCode); !exists {
					break
				}
			}
			log.Info().Str("room-code", roomCode).Msg("Created room")
			room = NewRoom()
			s.RegisterCode(roomCode, room)
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
				s.RemoveCode(roomCode)
				return
			}

			closed := make(chan bool)
			ticker := time.NewTicker(time.Minute)

			go func() {
				sendReconnectionKey := func() {
					token, err := GenerateReconnectionJWT(roomCode)
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
					room.Lock()
					room.reconnected = make(chan bool)
					room.Unlock()
					disconnectMsg, _ := json.Marshal(map[string]string{"type": "hostDisconnected"})
					room.Broadcast(disconnectMsg)
					select {
					case <-room.reconnected:
						reconnectionTimer.Stop()
						room.Lock()
						room.reconnected = nil
						room.Unlock()
						reconnectedMsg, _ := json.Marshal(map[string]string{"type": "hostReconnected"})
						room.Broadcast(reconnectedMsg)
					case <-reconnectionTimer.C:
						s.RemoveCode(roomCode)
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
					room.Lock()
					room.videoState.UpdatePath(parsedMessage.Path)
					room.videoState.UpdateIsPaused(parsedMessage.IsPaused)
					room.videoState.UpdateRate(parsedMessage.Rate)
					room.videoState.UpdateTime(parsedMessage.Time)
					room.Unlock()
				case "startPlaying":
					var parsedMessage struct {
						Time float64 `json:"time"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.Lock()
					room.videoState.UpdateIsPaused(false)
					room.videoState.UpdateTime(parsedMessage.Time)
					room.Unlock()
				case "pause":
					var parsedMessage struct {
						Time float64 `json:"time"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.Lock()
					room.videoState.UpdateIsPaused(true)
					room.videoState.UpdateTime(parsedMessage.Time)
					room.Unlock()
				case "pathChange":
					var parsedMessage struct {
						Path string `json:"path"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.Lock()
					room.videoState.UpdatePath(parsedMessage.Path)
					room.Unlock()
				case "rateChange":
					var parsedMessage struct {
						Rate float32 `json:"rate"`
					}
					json.Unmarshal(msg, &parsedMessage)
					room.Lock()
					room.videoState.UpdateRate(parsedMessage.Rate)
					room.Unlock()
				case "removeRoom":
					ticker.Stop()
					closed <- true
					s.RemoveCode(roomCode)
					room.CloseReceivers()
					logger.Info().Msg("Removing room")
					return
				}
				room.Broadcast(msg)
			}
		}()
	})
	r.Get("/room/{roomCode}/path", func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "roomCode")
		room, exists := s.GetRoom(code)
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		room.RLock()
		path := room.videoState.Path
		room.RUnlock()
		w.Write([]byte(path))
	})
	r.Get("/room/{roomCode}", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, "HTTP Streaming not supported!", http.StatusBadRequest)
			return
		}
		code := chi.URLParam(r, "roomCode")
		room, exists := s.GetRoom(code)
		if !exists {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		room.RLock()
		initialData, _ := json.Marshal(struct {
			Type string `json:"type"`
			VideoState
		}{Type: "sync", VideoState: room.videoState.GetPredicted()})
		room.RUnlock()
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

	})

	log.Info().Msgf("Starting server on port %v", port)
	http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), r)
}
