package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/golang-jwt/jwt/v4"
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
	config := MustLoadConfig()
	server := NewServer()
	httpServer := NewHTTPServer(server)

	log.Info().Msgf("Starting server on port %v", config.Port)
	http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", config.Port), httpServer)
}
