package main

import (
	"slices"
	"sync"
)

type Room struct {
	receivers   []chan []byte
	reconnected chan bool
	videoState  VideoState
	sync.RWMutex
}

func NewRoom() *Room {
	return &Room{receivers: make([]chan []byte, 0), reconnected: nil, videoState: VideoState{Path: "", Time: 0, Rate: 1, IsPaused: false}}
}

func (r *Room) IsHostConnected() bool {
	r.RLock()
	defer r.RUnlock()
	return r.reconnected == nil
}

func (r *Room) Join(newChan chan []byte) {
	r.Lock()
	r.receivers = append(r.receivers, newChan)
	r.Unlock()
}

func (r *Room) Leave(channel chan []byte) {
	r.Lock()
	for i, receiver := range r.receivers {
		if receiver == channel {
			r.receivers = slices.Delete(r.receivers, i, i+1)
			break
		}
	}
	r.Unlock()
}

func (r *Room) Broadcast(message []byte) {
	r.RLock()
	for _, receiver := range r.receivers {
		receiver <- message
	}
	r.RUnlock()
}

func (r *Room) CloseReceivers() {
	r.RLock()
	for _, receiver := range r.receivers {
		close(receiver)
	}
	r.RUnlock()
}
