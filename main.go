package main

import (
	"fmt"
	"net/http"
)

func main() {
	config := MustLoadConfig()
	server := NewServer()
	reconnectJWT := NewReconnect(config.JwtSecret, server)
	httpServer := NewHTTPServer(server, reconnectJWT)

	LogStartedServer(config.Port)
	http.ListenAndServe(fmt.Sprintf("127.0.0.1:%v", config.Port), httpServer)
}
