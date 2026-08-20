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
	"io"
	"strconv"
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
//
// TODO: implement this.
//
// Steps:
//  1. Read a single byte — this is the type prefix.
//  2. Switch on it and dispatch to the matching read*/parse* helper below.
//  3. If the byte doesn't match any known Type, return an error wrapping
//     ErrProtocol (don't panic, don't silently ignore it — a real client
//     could send anything).
//
// Hint: r.r.ReadByte() reads exactly one byte and is the right tool here
// — don't use ReadString or Scanner for the type prefix.
func (r *Reader) Read() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
}

// readLine reads bytes up to and including "\r\n" and returns the line
// WITHOUT the trailing \r\n.
//
// TODO: implement this.
//
// This is used for every RESP line that ISN'T raw binary payload
// (i.e. everything except the actual bytes of a bulk string body):
// the first line of a simple string, error, integer, bulk-string length
// header, and array length header.
//
// Hint: bufio.Reader has a ReadString method. Read up to '\n', then trim
// both '\r' and '\n' off the end. Watch out for a client that sends a
// line with no trailing \r before \n — decide how strict you want to be
// and note the tradeoff in a comment.
func (r *Reader) readLine() (string, error) {
	// TODO: implement
	return "", errors.New("not implemented")
}

// readSimpleString parses everything after a '+' prefix.
//
// TODO: implement this. It's the simplest case — one line, no length
// prefix, no binary data. Use it to sanity-check your readLine before
// tackling the harder cases below.
func (r *Reader) readSimpleString() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
}

// readError parses everything after a '-' prefix. Structurally
// identical to readSimpleString but tagged as Type Error.
//
// TODO: implement this.
func (r *Reader) readError() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
}

// readInteger parses everything after a ':' prefix into Value.Num.
//
// TODO: implement this.
//
// Hint: strconv.ParseInt(line, 10, 64). RESP integers can be negative,
// so don't assume unsigned.
func (r *Reader) readInteger() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
}

// readBulkString parses everything after a '$' prefix.
//
// TODO: implement this. This is the important one — get it right and
// arrays are easy, since arrays of bulk strings are how every real
// command arrives.
//
// Steps:
//  1. Read the length line (e.g. "6"). Parse it as an int.
//  2. Special case: length == -1 means a null bulk string. Return
//     Value{Type: BulkString, IsNull: true}, nil — do NOT try to read
//     a body.
//  3. Otherwise, read EXACTLY that many bytes as the payload, using
//     io.ReadFull (NOT readLine/ReadString!). Bulk string payloads are
//     arbitrary bytes and can legally contain \r, \n, or any byte value
//     — that's the whole point of length-prefixing instead of using a
//     delimiter. Using a line-based read here is the single most common
//     bug in hand-rolled RESP parsers.
//  4. After the payload, the wire still has a trailing "\r\n" you must
//     consume (and can discard) before returning, so the stream is
//     correctly positioned for whatever gets read next.
func (r *Reader) readBulkString() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
}

// readArray parses everything after a '*' prefix.
//
// TODO: implement this last — it depends on the other four.
//
// Steps:
//  1. Read the count line, parse as int.
//  2. Special case: count == -1 means a null array. Return
//     Value{Type: Array, IsNull: true}, nil.
//  3. Special case: count == 0 is a valid EMPTY array — not an error,
//     and not the same as null. Return an Array Value with an empty
//     (non-nil, if you want the distinction to matter) slice.
//  4. Otherwise, call r.Read() exactly `count` times, recursively — each
//     element can in principle be any RESP type, though in this project
//     you'll only ever receive arrays of bulk strings from redis-cli.
//     Append each parsed Value to the array.
func (r *Reader) readArray() (Value, error) {
	// TODO: implement
	return Value{}, errors.New("not implemented")
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
