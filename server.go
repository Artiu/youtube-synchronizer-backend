package main

import "sync"

type Server struct {
	codes map[string]*Room
	sync.RWMutex
}

func NewServer() *Server {
	return &Server{codes: make(map[string]*Room)}
}

func (s *Server) RegisterCode(code string, room *Room) {
	s.Lock()
	s.codes[code] = room
	s.Unlock()
}

func (s *Server) RemoveCode(code string) {
	s.Lock()
	delete(s.codes, code)
	s.Unlock()
}

func (s *Server) GetRoom(code string) (*Room, bool) {
	s.RLock()
	room, exists := s.codes[code]
	s.RUnlock()
	return room, exists
}
