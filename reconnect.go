package main

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const reconnectionTime = time.Minute * 2
const reconnectionTokenSendFreq = time.Minute

type Reconnect struct {
	jwtSecret string
	server    *Server
}

func NewReconnect(jwtSecret string, server *Server) *Reconnect {
	return &Reconnect{jwtSecret, server}
}

func (r Reconnect) GenerateReconnectionToken(roomCode string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"roomCode": roomCode, "exp": jwt.NewNumericDate(time.Now().Add(reconnectionTime))})
	return token.SignedString([]byte(r.jwtSecret))
}

func (r Reconnect) getRoomCode(tokenString string) string {
	token, _ := jwt.Parse(tokenString, func(t *jwt.Token) (interface{}, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return []byte(r.jwtSecret), nil
	})
	if claims, ok := token.Claims.(jwt.MapClaims); ok && token.Valid {
		return claims["roomCode"].(string)
	}
	return ""
}

func (r Reconnect) Try(reconnectionKey string) (string, *Room, error) {
	if reconnectionKey == "" {
		return "", nil, errors.New("empty reconnection key")
	}
	roomCode := r.getRoomCode(reconnectionKey)
	if roomCode == "" {
		return "", nil, errors.New("incorrect or expired JWT")
	}
	room, _ := r.server.GetRoom(roomCode)
	if room == nil {
		room = NewRoom()
		r.server.RegisterCode(roomCode, room)
		return roomCode, room, nil
	}
	if room.IsHostConnected() {
		return "", nil, errors.New("host already connected")
	}
	room.SendHostReconnected()
	return roomCode, room, nil
}
