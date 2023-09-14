package main

import (
	"fmt"
	"net/http"

	"github.com/rs/zerolog/log"
)

func main() {
	config := MustLoadConfig()
	server := NewServer()
	reconnectJWT := NewReconnectJWT(config.JwtSecret)
	httpServer := NewHTTPServer(server, reconnectJWT)

	log.Info().Msgf("Starting server on port %v", config.Port)
	http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", config.Port), httpServer)
}
