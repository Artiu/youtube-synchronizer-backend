package main

import (
	"encoding/json"
	"fmt"
	"net/http"
)

type ReceiverSSE struct {
	w http.ResponseWriter
	f http.Flusher
}

func NewReceiverSSE(w http.ResponseWriter, f http.Flusher) *ReceiverSSE {
	return &ReceiverSSE{w, f}
}

func (r ReceiverSSE) SendByteSlice(msg []byte) {
	fmt.Fprintf(r.w, "data: %v\n\n", string(msg))
	r.f.Flush()
}

func (r ReceiverSSE) sendJSON(msg any) {
	data, _ := json.Marshal(msg)
	r.SendByteSlice(data)
}

func (r ReceiverSSE) SendSyncMessage(videoState VideoState) {
	r.sendJSON(struct {
		Type string `json:"type"`
		VideoState
	}{Type: "sync", VideoState: videoState})
}

func (r ReceiverSSE) SendHostDisconnectedMessage() {
	r.sendJSON(struct {
		Type string `json:"type"`
	}{Type: "hostDisconnected"})
}

func (r ReceiverSSE) SendRoomClosedMessage() {
	r.sendJSON(struct {
		Type string `type:"type"`
	}{Type: "close"})
}
