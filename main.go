package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"sync"
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

type VideoState struct {
	Path     *string  `json:"path"`
	Time     *float64 `json:"time"`
	Rate     *float32 `json:"rate"`
	IsPaused *bool    `json:"isPaused"`
}

type Room struct {
	receivers   []chan []byte
	reconnected chan bool
	videoState  VideoState
	sync.RWMutex
}

func NewRoom() *Room {
	return &Room{receivers: make([]chan []byte, 0), reconnected: make(chan bool, 1)}
}

func (r *Room) Join(newChan chan []byte) {
	r.Lock()
	r.receivers = append(r.receivers, newChan)
	initialData, _ := json.Marshal(struct {
		Type string `json:"type"`
		VideoState
	}{Type: "sync", VideoState: r.videoState})
	newChan <- initialData
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

func (r *Room) UpdateVideoState(newVideoState VideoState) {
	r.Lock()
	if newVideoState.Path != nil {
		r.videoState.Path = newVideoState.Path
	}
	if newVideoState.IsPaused != nil {
		r.videoState.IsPaused = newVideoState.IsPaused
	}
	if newVideoState.Rate != nil {
		r.videoState.Rate = newVideoState.Rate
	}
	if newVideoState.Time != nil {
		r.videoState.Time = newVideoState.Time
	}
	r.Unlock()
}

type Server struct {
	codes map[string]*Room
	sync.RWMutex
}

func NewServer() *Server {
	return &Server{codes: make(map[string]*Room)}
}

func (s *Server) RegisterCode(code string, room *Room) {
	s.Lock()
	s.codes[code] = room
	s.Unlock()
}

func (s *Server) RemoveCode(code string) {
	s.Lock()
	delete(s.codes, code)
	s.Unlock()
}

func (s *Server) GetRoom(code string) (*Room, bool) {
	s.RLock()
	room, exists := s.codes[code]
	s.RUnlock()
	return room, exists
}

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
		AllowedOrigins:   []string{"chrome-extension://*"},
		AllowedMethods:   []string{"GET"},
		AllowCredentials: false,
	}))
	r.Use(middleware.RealIP)
	r.Use(httprate.LimitByIP(10, time.Minute))
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
		} else {
			log.Info().Str("room-code", roomCode).Msg("Reconnected")
			room.reconnected <- true
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
				var newVideoState VideoState
				json.Unmarshal(msg, &newVideoState)
				room.UpdateVideoState(newVideoState)
				if err != nil {
					logger.Info().Msg("Disconnected")
					ticker.Stop()
					closed <- true
					reconnectionTimer := time.NewTimer(reconnectionTime)
					select {
					case <-room.reconnected:
						reconnectionTimer.Stop()
					case <-reconnectionTimer.C:
						s.RemoveCode(roomCode)
						room.CloseReceivers()
						logger.Info().Msg("Removing room")
					}
					break
				}
				room.Broadcast(msg)
			}
		}()
	})
	r.Get("/room/{roomCode}", func(w http.ResponseWriter, r *http.Request) {
		code := chi.URLParam(r, "roomCode")
		room, exists := s.GetRoom(code)
		if !exists {
			w.WriteHeader(404)
			return
		}
		sendChannel := make(chan []byte, 1)
		room.Join(sendChannel)
		logger := GetLogger(r.RemoteAddr, code)
		logger.Info().Msg("Joined room")
		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("Connection", "keep-alive")
		pingTimer := time.NewTicker(time.Second * 15)
	messageLoop:
		for {
			select {
			case <-pingTimer.C:
				fmt.Fprint(w, ":\n\n")
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case msg, more := <-sendChannel:
				if !more {
					pingTimer.Stop()
					room.Leave(sendChannel)
					logger.Info().Msg("Left room")
					break messageLoop
				}
				fmt.Fprintf(w, "data: %v\n\n", string(msg))
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			case <-r.Context().Done():
				pingTimer.Stop()
				room.Leave(sendChannel)
				logger.Info().Err(r.Context().Err()).Msg("Left room")
				break messageLoop
			}
		}

	})

	log.Info().Msgf("Starting server on port %v", port)
	http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", port), r)
}
