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

func (r ReceiverSSE) SendSyncMessage(videoState VideoState) {
	data, _ := json.Marshal(struct {
		Type string `json:"type"`
		VideoState
	}{Type: "sync", VideoState: videoState})
	r.SendByteSlice(data)
}

func (r ReceiverSSE) SendHostDisconnectedMessage() {
	data, _ := json.Marshal(struct {
		Type string `json:"type"`
	}{Type: "hostDisconnected"})
	r.SendByteSlice(data)
}

func (r ReceiverSSE) SendRoomClosedMessage() {
	data, _ := json.Marshal(struct {
		Type string `type:"type"`
	}{Type: "close"})
	r.SendByteSlice(data)
}
