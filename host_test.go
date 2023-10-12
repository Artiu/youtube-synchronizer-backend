package main

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/gobwas/ws/wsutil"
)

func TestSendRoomCode(t *testing.T) {
	client, server := net.Pipe()
	go func() {
		hostWs := NewHostWebsocket(server)
		hostWs.SendRoomCode("amazingRoom")
		server.Close()
	}()
	data, _ := wsutil.ReadServerText(client)
	var parsed CodeMessage
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		t.Errorf("incorrect json sent")
	}
	if parsed.Type != "code" {
		t.Errorf("wrong type expected: %v got: %v", "roomCode", parsed.Type)
	}
	if parsed.Code != "amazingRoom" {
		t.Errorf("wrong code expected: %v got: %v", "amazingRoom", parsed.Code)
	}
	client.Close()
}
