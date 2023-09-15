package main

import (
	"encoding/json"
	"slices"
	"sync"
)

type Room struct {
	receivers   []chan []byte
	reconnected chan bool
	videoState  VideoState
	lock        sync.RWMutex
}

func NewRoom() *Room {
	return &Room{receivers: make([]chan []byte, 0), reconnected: nil, videoState: VideoState{Path: "", Time: 0, Rate: 1, IsPaused: false}}
}

func (r *Room) IsHostConnected() bool {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.reconnected == nil
}

func (r *Room) SendHostReconnected() {
	r.lock.RLock()
	defer r.lock.RUnlock()
	r.reconnected <- true
}

func (r *Room) GetVideoPath() string {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.videoState.Path
}

func (r *Room) updateReconnectChannel(newReconnectChannel chan bool) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.reconnected = newReconnectChannel
}

func (r *Room) HostDisconnected() {
	type HostDisconnectedMsg struct {
		Type string `json:"type"`
	}
	r.updateReconnectChannel(make(chan bool))
	disconnectMsg, _ := json.Marshal(HostDisconnectedMsg{"hostDisconnected"})
	r.broadcast(disconnectMsg)
}

func (r *Room) HostReconnected() {
	type HostReconnectedMsg struct {
		Type string `json:"type"`
	}
	r.updateReconnectChannel(nil)
	reconnectedMsg, _ := json.Marshal(HostReconnectedMsg{"hostReconnected"})
	r.broadcast(reconnectedMsg)
}

// Pass only message which are one of structs of Message interface
func (r *Room) HostMessage(msg any) {
	encoded, _ := json.Marshal(msg)
	r.broadcast(encoded)
}

func (r *Room) UpdateVideoState(updateFunc func(v *VideoState)) {
	r.lock.Lock()
	defer r.lock.Unlock()
	updateFunc(&r.videoState)
}

func (r *Room) GetPredictedVideoState() VideoState {
	r.lock.RLock()
	defer r.lock.RUnlock()
	return r.videoState.GetPredicted()
}

func (r *Room) Join(newChan chan []byte) {
	r.lock.Lock()
	defer r.lock.Unlock()
	r.receivers = append(r.receivers, newChan)
}

func (r *Room) Leave(channel chan []byte) {
	r.lock.Lock()
	defer r.lock.Unlock()
	for i, receiver := range r.receivers {
		if receiver == channel {
			r.receivers = slices.Delete(r.receivers, i, i+1)
			break
		}
	}
}

func (r *Room) broadcast(message []byte) {
	r.lock.RLock()
	defer r.lock.RUnlock()
	for _, receiver := range r.receivers {
		receiver <- message
	}
}

func (r *Room) CloseReceivers() {
	r.lock.RLock()
	defer r.lock.RUnlock()
	for _, receiver := range r.receivers {
		close(receiver)
	}
}
