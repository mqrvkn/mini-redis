// Package server implements the TCP front-end. Week 1 uses a simple
// space-separated, newline-terminated text protocol. Week 2 replaces the
// parsing in dispatch/handleConn with real RESP — the Store and the
// connection-loop concurrency model underneath don't change.
package server

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"strings"

	"mini-redis/internal/store"
)

// Server holds shared state for all connections.
type Server struct {
	addr string
	db   *store.Store
}

// New creates a Server listening on addr (e.g. ":6380").
func New(addr string, db *store.Store) *Server {
	return &Server{addr: addr, db: db}
}

// ListenAndServe starts accepting connections and blocks until the
// listener errors (e.g. port already in use).
func (s *Server) ListenAndServe() error {
	ln, err := net.Listen("tcp", s.addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", s.addr, err)
	}
	defer ln.Close()

	log.Printf("mini-redis listening on %s", s.addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("accept error: %v", err)
			continue
		}
		go s.handleConn(conn)
	}
}

// handleConn owns one client connection for its whole lifetime.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr()
	log.Printf("client connected: %s", remote)
	defer log.Printf("client disconnected: %s", remote)

	reader := bufio.NewReader(conn)

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}

		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			continue
		}

		response := s.dispatch(line)
		if _, err := conn.Write([]byte(response + "\r\n")); err != nil {
			log.Printf("write error to %s: %v", remote, err)
			return
		}
	}
}

// dispatch parses one line into a command and arguments and executes it.
func (s *Server) dispatch(line string) string {
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "-ERR empty command"
	}

	cmd := strings.ToUpper(parts[0])
	args := parts[1:]

	switch cmd {
	case "PING":
		return "+PONG"

	case "SET":
		if len(args) != 2 {
			return "-ERR usage: SET key value"
		}
		s.db.Set(args[0], args[1])
		return "+OK"

	case "GET":
		if len(args) != 1 {
			return "-ERR usage: GET key"
		}
		v, ok := s.db.Get(args[0])
		if !ok {
			return "$-1"
		}
		return "$" + v

	case "DEL":
		if len(args) != 1 {
			return "-ERR usage: DEL key"
		}
		if s.db.Del(args[0]) {
			return ":1"
		}
		return ":0"

	case "EXISTS":
		if len(args) != 1 {
			return "-ERR usage: EXISTS key"
		}
		if s.db.Exists(args[0]) {
			return ":1"
		}
		return ":0"

	default:
		return "-ERR unknown command '" + cmd + "'"
	}
}
