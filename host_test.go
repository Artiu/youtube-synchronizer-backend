package main

import (
	"encoding/json"
	"net"
	"testing"

	"github.com/gobwas/ws/wsutil"
)

func TestSendRoomCode(t *testing.T) {
	client, server := net.Pipe()
	correctData := CodeMessage{Type: "code", Code: "amazingRoom"}
	go func() {
		hostWs := NewHostWebsocket(server)
		hostWs.SendRoomCode(correctData.Code)
		server.Close()
	}()
	data, _ := wsutil.ReadServerText(client)
	var parsed CodeMessage
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		t.Errorf("incorrect json sent")
	}
	if parsed != correctData {
		t.Errorf("wrong struct received expected: %v got: %v", correctData, parsed)
	}
	client.Close()
}

func TestSendReconnectKey(t *testing.T) {
	client, server := net.Pipe()
	correctData := ReconnectKeyMessage{Type: "reconnectKey", ReconnectKey: "superSecretKey"}
	go func() {
		hostWs := NewHostWebsocket(server)
		hostWs.SendReconnectKey(correctData.ReconnectKey)
		server.Close()
	}()
	data, _ := wsutil.ReadServerText(client)
	var parsed ReconnectKeyMessage
	err := json.Unmarshal(data, &parsed)
	if err != nil {
		t.Errorf("incorrect json sent")
	}
	if parsed != correctData {
		t.Errorf("wrong struct received expected: %v got: %v", correctData, parsed)
	}
	client.Close()
}

func TestReadMessage(t *testing.T) {
	type test map[string]struct {
		input map[string]any
		want  any
	}
	tests := test{
		"SyncMessage": {input: map[string]any{"type": "sync", "path": "/", "time": 0.25, "rate": 1, "isPaused": true}, want: SyncMessage{Path: "/", Time: 0.25, Rate: 1, IsPaused: true}},
		"WrongType":   {input: map[string]any{"type": "notExistingType"}, want: ErrUndefinedType},
	}
	for name, tst := range tests {
		t.Run(name, func(t *testing.T) {
			client, server := net.Pipe()
			go func() {
				data, _ := json.Marshal(tst.input)
				wsutil.WriteClientText(client, data)
				client.Close()
			}()
			hostWs := NewHostWebsocket(server)
			receivedMsg, err := hostWs.ReadMessage()
			wantErr, expectErr := tst.want.(error)
			if expectErr {
				if err != wantErr {
					t.Errorf("wrong error expected: %v, got: %v", wantErr, err)
				}
				return
			}
			if err != nil {
				t.Errorf("error %v while reading: %v", err, tst)
			}
			if receivedMsg != tst.want {
				t.Errorf("expected: %v, got: %v", tst, receivedMsg)
			}
			server.Close()
		})
	}

}
