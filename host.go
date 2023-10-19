package main

import (
	"encoding/json"
	"errors"
	"net"

	"github.com/gobwas/ws/wsutil"
)

type HostWebsocket struct {
	conn net.Conn
}

type CodeMessage struct {
	Type string `json:"type"`
	Code string `json:"code"`
}

type ReconnectKeyMessage struct {
	Type         string `json:"type"`
	ReconnectKey string `json:"reconnectKey"`
}

func UnmarshalJSON[T any](data []byte) T {
	var parsed T
	json.Unmarshal(data, &parsed)
	return parsed
}

func NewHostWebsocket(conn net.Conn) *HostWebsocket {
	return &HostWebsocket{conn}
}

func (h HostWebsocket) sendMessage(message any) error {
	encoded, _ := json.Marshal(message)
	return wsutil.WriteServerText(h.conn, encoded)
}

func (h HostWebsocket) SendRoomCode(roomCode string) error {
	return h.sendMessage(CodeMessage{Type: "code", Code: roomCode})
}

func (h HostWebsocket) SendReconnectKey(reconnectKey string) error {
	return h.sendMessage(ReconnectKeyMessage{Type: "reconnectKey", ReconnectKey: reconnectKey})
}

type SyncMessage struct {
	Path     string  `json:"path"`
	Time     float64 `json:"time"`
	Rate     float32 `json:"rate"`
	IsPaused bool    `json:"isPaused"`
}

type StartPlayingMessage struct {
	Time float64 `json:"time"`
}

type PauseMessage struct {
	Time float64 `json:"time"`
}

type PathChangeMessage struct {
	Path string `json:"path"`
}

type RateChangeMessage struct {
	Rate float32 `json:"rate"`
}

type RemoveRoomMessage struct{}

type Message interface {
	SyncMessage | StartPlayingMessage | PauseMessage | PathChangeMessage | RateChangeMessage | RemoveRoomMessage
}

var ErrUndefinedType = errors.New("incorrect type")

// Returns one of struct from message interface
func (h HostWebsocket) ReadMessage() (any, error) {
	msg, err := wsutil.ReadClientText(h.conn)
	if err != nil {
		return nil, err
	}
	message := UnmarshalJSON[struct {
		Type string `json:"type"`
	}](msg)
	var parsedMessage any
	switch message.Type {
	case "sync":
		parsedMessage = UnmarshalJSON[SyncMessage](msg)
	case "startPlaying":
		parsedMessage = UnmarshalJSON[StartPlayingMessage](msg)
	case "pause":
		parsedMessage = UnmarshalJSON[PauseMessage](msg)
	case "pathChange":
		parsedMessage = UnmarshalJSON[PathChangeMessage](msg)
	case "rateChange":
		parsedMessage = UnmarshalJSON[RateChangeMessage](msg)
	case "removeRoom":
		parsedMessage = RemoveRoomMessage{}
	default:
		return nil, ErrUndefinedType
	}
	return parsedMessage, nil
}
