package main

import (
	"sync"
)

type Server struct {
	codes map[string]*Room
	lock  sync.RWMutex
}

func NewServer() *Server {
	return &Server{codes: make(map[string]*Room)}
}

func (s *Server) RegisterCode(code string, room *Room) {
	s.lock.Lock()
	defer s.lock.Unlock()
	s.codes[code] = room
}

func (s *Server) RemoveCode(code string) {
	s.lock.Lock()
	defer s.lock.Unlock()
	delete(s.codes, code)
}

func (s *Server) GetRoom(code string) (*Room, bool) {
	s.lock.RLock()
	defer s.lock.RUnlock()
	room, exists := s.codes[code]
	return room, exists
}
