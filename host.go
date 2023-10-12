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

func NewHostWebsocket(conn net.Conn) *HostWebsocket {
	return &HostWebsocket{conn}
}

func (h HostWebsocket) sendMessage(message []byte) error {
	return wsutil.WriteServerText(h.conn, message)
}

func (h HostWebsocket) SendRoomCode(roomCode string) error {
	encoded, _ := json.Marshal(CodeMessage{"code", roomCode})
	return h.sendMessage(encoded)
}

func (h HostWebsocket) SendReconnectKey(reconnectKey string) error {
	type ReconnectKeyMessage struct {
		Type         string `json:"type"`
		ReconnectKey string `json:"reconnectKey"`
	}
	encoded, _ := json.Marshal(ReconnectKeyMessage{"reconnectKey", reconnectKey})
	return h.sendMessage(encoded)
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
