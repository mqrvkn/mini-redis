// Package resp implements the RESP (REdis Serialization Protocol) wire
// format, so this server can talk to real redis-cli and real Redis
// client libraries.
//
// RESP encodes every value with a one-byte type prefix, then a
// CRLF-terminated ("\r\n") payload. The five types you need for a
// command-response protocol:
//
//	Simple String   +OK\r\n
//	Error            -ERR something went wrong\r\n
//	Integer          :1000\r\n
//	Bulk String      $6\r\nfoobar\r\n        (length-prefixed, binary safe)
//	Null Bulk String $-1\r\n                  (means "nil" / missing key)
//	Array            *2\r\n$3\r\nfoo\r\n$3\r\nbar\r\n
//	Null Array       *-1\r\n
//
// A command from redis-cli always arrives as an Array of Bulk Strings,
// e.g. `SET foo bar` is sent over the wire as:
//
//	*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n
//
// Your job in this file is Read() and its helpers — parsing bytes off
// the wire into a Value. Marshal() (Value -> bytes, for sending
// responses) is done for you as a reference for the format.
package resp

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// Type is the one-byte RESP type prefix.
type Type byte

const (
	SimpleString Type = '+'
	Error        Type = '-'
	Integer      Type = ':'
	BulkString   Type = '$'
	Array        Type = '*'
)

// ErrProtocol indicates malformed input that doesn't follow RESP framing.
// Wrap parsing failures in this so callers can distinguish "client sent
// garbage" from "connection closed" (io.EOF) or other I/O errors.
var ErrProtocol = errors.New("resp: protocol error")

// Value is a parsed RESP value. Only the fields relevant to Type are
// meaningful — e.g. for a BulkString, Str and IsNull matter; Array and
// Num are unused.
type Value struct {
	Type   Type
	Str    string  // SimpleString, Error, BulkString payload
	Num    int64   // Integer payload
	Array  []Value // Array elements
	IsNull bool    // true for null bulk string ($-1) or null array (*-1)
}

// --- Constructors for building responses (done for you) ---

func SimpleStringValue(s string) Value { return Value{Type: SimpleString, Str: s} }
func ErrorValue(s string) Value        { return Value{Type: Error, Str: s} }
func IntegerValue(n int64) Value       { return Value{Type: Integer, Num: n} }
func BulkStringValue(s string) Value   { return Value{Type: BulkString, Str: s} }
func NullBulkString() Value            { return Value{Type: BulkString, IsNull: true} }
func ArrayValue(vs []Value) Value      { return Value{Type: Array, Array: vs} }
func NullArray() Value                 { return Value{Type: Array, IsNull: true} }

// --- Reader: bytes -> Value ---

// Reader parses RESP values from a stream.
type Reader struct {
	r *bufio.Reader
}

// NewReader wraps rd for RESP parsing. Callers typically pass a
// net.Conn directly; Reader does its own buffering.
func NewReader(rd io.Reader) *Reader {
	return &Reader{r: bufio.NewReader(rd)}
}

// Read parses one complete RESP value from the stream.
func (r *Reader) Read() (Value, error) {
	typeByte, err := r.r.ReadByte()
	if err != nil {
		return Value{}, err
	}

	switch Type(typeByte) {
	case SimpleString:
		return r.readSimpleString()
	case Error:
		return r.readError()
	case Integer:
		return r.readInteger()
	case BulkString:
		return r.readBulkString()
	case Array:
		return r.readArray()
	default:
		return Value{}, fmt.Errorf("%w: unknown type prefix %q", ErrProtocol, typeByte)
	}
}

// readLine reads bytes up to and including "\r\n" and returns the line
// WITHOUT the trailing \r\n.
//
// This is used for every RESP line that ISN'T raw binary payload
// (i.e. everything except the actual bytes of a bulk string body):
// the first line of a simple string, error, integer, bulk-string length
// header, and array length header.
func (r *Reader) readLine() (string, error) {
	line, err := r.r.ReadString('\n')
	if err != nil {
		return "", err
	}
	// Design choice: TrimRight strips a trailing \r AND \n regardless of
	// whether the \r is actually present. Strict RESP always sends \r\n,
	// but being lenient here costs nothing and tolerates a client (or a
	// human typing into `nc`) that only sends \n. A stricter parser
	// could instead check for an exact "\r\n" suffix and return
	// ErrProtocol if the \r is missing — worth naming as a tradeoff.
	line = strings.TrimRight(line, "\r\n")
	return line, nil
}

// readSimpleString parses everything after a '+' prefix.
func (r *Reader) readSimpleString() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{Type: SimpleString, Str: line}, nil
}

// readError parses everything after a '-' prefix. Structurally
// identical to readSimpleString but tagged as Type Error.
func (r *Reader) readError() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	return Value{Type: Error, Str: line}, nil
}

// readInteger parses everything after a ':' prefix into Value.Num.
func (r *Reader) readInteger() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	n, err := strconv.ParseInt(line, 10, 64)
	if err != nil {
		return Value{}, fmt.Errorf("%w: invalid integer %q", ErrProtocol, line)
	}
	return Value{Type: Integer, Num: n}, nil
}

// readBulkString parses everything after a '$' prefix.
func (r *Reader) readBulkString() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	length, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("%w: invalid bulk string length %q", ErrProtocol, line)
	}

	if length == -1 {
		return Value{Type: BulkString, IsNull: true}, nil
	}

	// Read EXACTLY `length` bytes — not a line. Payloads can legally
	// contain \r or \n in the middle; io.ReadFull respects the byte
	// count regardless of what those bytes are, which is the whole
	// point of length-prefixing instead of delimiter-based framing.
	payload := make([]byte, length)
	if _, err := io.ReadFull(r.r, payload); err != nil {
		return Value{}, err
	}

	// Consume and discard the trailing \r\n after the payload so the
	// stream is correctly positioned for the next Read().
	trailer := make([]byte, 2)
	if _, err := io.ReadFull(r.r, trailer); err != nil {
		return Value{}, err
	}

	return Value{Type: BulkString, Str: string(payload)}, nil
}

// readArray parses everything after a '*' prefix.
func (r *Reader) readArray() (Value, error) {
	line, err := r.readLine()
	if err != nil {
		return Value{}, err
	}
	count, err := strconv.Atoi(line)
	if err != nil {
		return Value{}, fmt.Errorf("%w: invalid array length %q", ErrProtocol, line)
	}

	if count == -1 {
		return Value{Type: Array, IsNull: true}, nil
	}

	elements := make([]Value, 0, count)
	for i := 0; i < count; i++ {
		elem, err := r.Read()
		if err != nil {
			return Value{}, err
		}
		elements = append(elements, elem)
	}

	return Value{Type: Array, Array: elements}, nil
}

// --- Writer: Value -> bytes (reference implementation, no changes needed) ---

// Marshal encodes a Value back into its wire representation.
func (v Value) Marshal() []byte {
	switch v.Type {
	case SimpleString:
		return []byte("+" + v.Str + "\r\n")
	case Error:
		return []byte("-" + v.Str + "\r\n")
	case Integer:
		return []byte(":" + strconv.FormatInt(v.Num, 10) + "\r\n")
	case BulkString:
		if v.IsNull {
			return []byte("$-1\r\n")
		}
		return []byte("$" + strconv.Itoa(len(v.Str)) + "\r\n" + v.Str + "\r\n")
	case Array:
		if v.IsNull {
			return []byte("*-1\r\n")
		}
		out := []byte("*" + strconv.Itoa(len(v.Array)) + "\r\n")
		for _, elem := range v.Array {
			out = append(out, elem.Marshal()...)
		}
		return out
	default:
		// Unreachable if Values are only ever built via the constructors
		// above, but fail loudly rather than silently emit nothing.
		return []byte("-ERR internal: unknown value type\r\n")
	}
}
