package main

import (
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

const reconnectionTime = time.Minute * 2

type ReconnectJWT struct {
	jwtSecret string
}

func NewReconnectJWT(jwtSecret string) *ReconnectJWT {
	return &ReconnectJWT{jwtSecret}
}

func (r ReconnectJWT) GenerateReconnectionJWT(roomCode string) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{"roomCode": roomCode, "exp": jwt.NewNumericDate(time.Now().Add(reconnectionTime))})
	return token.SignedString([]byte(r.jwtSecret))
}

func (r ReconnectJWT) GetRoomCodeFromReconnectionJWT(tokenString string) string {
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
