package main

import (
	"time"
)

type Servers struct {
	s map[string]*Server
}

func NewServers() *Servers {
	return &Servers{
		s: map[string]*Server{},
	}
}

func (s *Servers) Exists(serverId string) bool {
	_, ok := s.s[serverId]
	return ok
}

func (s *Servers) Get(serverId string) *Server {
	return s.s[serverId]
}

func (s *Servers) Add(server *Server) *Server {
	s.s[server.Id] = server
	return server
}

func (s *Servers) Del(serverId string) {
	delete(s.s, serverId)
}

func (s *Servers) PortsReserved() []int {
	ports := []int{}
	for _, server := range s.s {
		ports = append(ports, server.Port)
	}
	return ports
}

func (s *Servers) Tick() {
	minute := time.NewTicker(60 * time.Second)
	for _ = range minute.C {
		for _, server := range s.s {
			if server.State == "up" {
				pushPinkyEvent(map[string]interface{}{
					"ts":        time.Now(),
					"server_id": server.Id,
					"event":     "server_event",
					"type":      "minute",
				})
			}
		}
	}
}
