// Package server implements the TCP front-end. As of Week 2, it speaks
// real RESP (see internal/resp) so redis-cli and real Redis client
// libraries can connect directly.
package server

import (
	"fmt"
	"log"
	"net"
	"strings"

	"mini-redis/internal/resp"
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

// handleConn owns one client connection for its whole lifetime. It reads
// RESP-encoded commands and writes RESP-encoded responses.
func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	remote := conn.RemoteAddr()
	log.Printf("client connected: %s", remote)
	defer log.Printf("client disconnected: %s", remote)

	reader := resp.NewReader(conn)

	for {
		val, err := reader.Read()
		if err != nil {
			// Includes io.EOF on clean disconnect, plus any protocol
			// error from a malformed client — either way, nothing more
			// can be reliably parsed from this connection.
			return
		}

		response := s.dispatch(val)
		if _, err := conn.Write(response.Marshal()); err != nil {
			log.Printf("write error to %s: %v", remote, err)
			return
		}
	}
}

// dispatch takes one parsed RESP value — expected to be an Array of
// BulkStrings, exactly what redis-cli sends for every command — and
// returns the RESP response to write back.
func (s *Server) dispatch(val resp.Value) resp.Value {
	if val.Type != resp.Array || len(val.Array) == 0 {
		return resp.ErrorValue("ERR expected command as array of bulk strings")
	}

	// Pull the command name and arguments out of the array.
	parts := make([]string, len(val.Array))
	for i, elem := range val.Array {
		if elem.Type != resp.BulkString || elem.IsNull {
			return resp.ErrorValue("ERR command elements must be bulk strings")
		}
		parts[i] = elem.Str
	}

	cmd := strings.ToUpper(parts[0])
	args := parts[1:]

	switch cmd {
	case "PING":
		return resp.SimpleStringValue("PONG")

	case "SET":
		if len(args) != 2 {
			return resp.ErrorValue("ERR usage: SET key value")
		}
		s.db.Set(args[0], args[1])
		return resp.SimpleStringValue("OK")

	case "GET":
		if len(args) != 1 {
			return resp.ErrorValue("ERR usage: GET key")
		}
		v, ok := s.db.Get(args[0])
		if !ok {
			return resp.NullBulkString()
		}
		return resp.BulkStringValue(v)

	case "DEL":
		if len(args) != 1 {
			return resp.ErrorValue("ERR usage: DEL key")
		}
		if s.db.Del(args[0]) {
			return resp.IntegerValue(1)
		}
		return resp.IntegerValue(0)

	case "EXISTS":
		if len(args) != 1 {
			return resp.ErrorValue("ERR usage: EXISTS key")
		}
		if s.db.Exists(args[0]) {
			return resp.IntegerValue(1)
		}
		return resp.IntegerValue(0)

	default:
		return resp.ErrorValue("ERR unknown command '" + cmd + "'")
	}
}
