package resp

import (
	"strings"
	"testing"
)

// --- Simple String ---

func TestReadSimpleString(t *testing.T) {
	r := NewReader(strings.NewReader("+OK\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != SimpleString {
		t.Fatalf("Type = %q, want SimpleString", v.Type)
	}
	if v.Str != "OK" {
		t.Fatalf("Str = %q, want %q", v.Str, "OK")
	}
}

// --- Error ---

func TestReadError(t *testing.T) {
	r := NewReader(strings.NewReader("-ERR unknown command\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != Error {
		t.Fatalf("Type = %q, want Error", v.Type)
	}
	if v.Str != "ERR unknown command" {
		t.Fatalf("Str = %q, want %q", v.Str, "ERR unknown command")
	}
}

// --- Integer ---

func TestReadInteger(t *testing.T) {
	cases := []struct {
		in   string
		want int64
	}{
		{":1000\r\n", 1000},
		{":0\r\n", 0},
		{":-1\r\n", -1},
	}
	for _, c := range cases {
		r := NewReader(strings.NewReader(c.in))
		v, err := r.Read()
		if err != nil {
			t.Fatalf("input %q: unexpected error: %v", c.in, err)
		}
		if v.Type != Integer {
			t.Fatalf("input %q: Type = %q, want Integer", c.in, v.Type)
		}
		if v.Num != c.want {
			t.Fatalf("input %q: Num = %d, want %d", c.in, v.Num, c.want)
		}
	}
}

// --- Bulk String ---

func TestReadBulkString(t *testing.T) {
	r := NewReader(strings.NewReader("$6\r\nfoobar\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != BulkString {
		t.Fatalf("Type = %q, want BulkString", v.Type)
	}
	if v.IsNull {
		t.Fatalf("IsNull = true, want false")
	}
	if v.Str != "foobar" {
		t.Fatalf("Str = %q, want %q", v.Str, "foobar")
	}
}

func TestReadBulkStringEmpty(t *testing.T) {
	r := NewReader(strings.NewReader("$0\r\n\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.IsNull {
		t.Fatalf("IsNull = true, want false (empty string is not null)")
	}
	if v.Str != "" {
		t.Fatalf("Str = %q, want empty string", v.Str)
	}
}

func TestReadBulkStringNull(t *testing.T) {
	r := NewReader(strings.NewReader("$-1\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != BulkString {
		t.Fatalf("Type = %q, want BulkString", v.Type)
	}
	if !v.IsNull {
		t.Fatalf("IsNull = false, want true")
	}
}

// This is the test that catches the most common hand-rolled-parser bug:
// treating bulk string payloads as line-delimited instead of respecting
// the length prefix. A payload can legally contain \r\n in the middle.
func TestReadBulkStringWithEmbeddedCRLF(t *testing.T) {
	payload := "foo\r\nbar" // 8 bytes, contains an embedded CRLF
	input := "$8\r\n" + payload + "\r\n"
	r := NewReader(strings.NewReader(input))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Str != payload {
		t.Fatalf("Str = %q, want %q — did you read by length instead of by line?", v.Str, payload)
	}
}

// --- Array ---

func TestReadArrayNull(t *testing.T) {
	r := NewReader(strings.NewReader("*-1\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != Array {
		t.Fatalf("Type = %q, want Array", v.Type)
	}
	if !v.IsNull {
		t.Fatalf("IsNull = false, want true")
	}
}

func TestReadArrayEmpty(t *testing.T) {
	r := NewReader(strings.NewReader("*0\r\n"))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.IsNull {
		t.Fatalf("IsNull = true, want false (empty array is not null)")
	}
	if len(v.Array) != 0 {
		t.Fatalf("len(Array) = %d, want 0", len(v.Array))
	}
}

// This is the shape every real command from redis-cli takes: an array
// of bulk strings. `SET foo bar` arrives exactly like this.
func TestReadArrayOfBulkStrings(t *testing.T) {
	input := "*3\r\n$3\r\nSET\r\n$3\r\nfoo\r\n$3\r\nbar\r\n"
	r := NewReader(strings.NewReader(input))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.Type != Array {
		t.Fatalf("Type = %q, want Array", v.Type)
	}
	if len(v.Array) != 3 {
		t.Fatalf("len(Array) = %d, want 3", len(v.Array))
	}
	want := []string{"SET", "foo", "bar"}
	for i, elem := range v.Array {
		if elem.Type != BulkString {
			t.Fatalf("Array[%d].Type = %q, want BulkString", i, elem.Type)
		}
		if elem.Str != want[i] {
			t.Fatalf("Array[%d].Str = %q, want %q", i, elem.Str, want[i])
		}
	}
}

func TestReadNestedArray(t *testing.T) {
	// *2\r\n *1\r\n $3\r\nfoo\r\n :5\r\n  ->  [["foo"], 5]
	input := "*2\r\n*1\r\n$3\r\nfoo\r\n:5\r\n"
	r := NewReader(strings.NewReader(input))
	v, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(v.Array) != 2 {
		t.Fatalf("len(Array) = %d, want 2", len(v.Array))
	}
	inner := v.Array[0]
	if inner.Type != Array || len(inner.Array) != 1 || inner.Array[0].Str != "foo" {
		t.Fatalf("Array[0] = %+v, want nested array [\"foo\"]", inner)
	}
	if v.Array[1].Type != Integer || v.Array[1].Num != 5 {
		t.Fatalf("Array[1] = %+v, want Integer 5", v.Array[1])
	}
}

// --- Multiple values back to back on one stream ---
// A real connection sends many commands in sequence; Read() must leave
// the stream correctly positioned for the next call each time.

func TestReadSequentialValues(t *testing.T) {
	input := "+OK\r\n:42\r\n$3\r\nfoo\r\n"
	r := NewReader(strings.NewReader(input))

	v1, err := r.Read()
	if err != nil || v1.Type != SimpleString || v1.Str != "OK" {
		t.Fatalf("first Read: v=%+v err=%v", v1, err)
	}
	v2, err := r.Read()
	if err != nil || v2.Type != Integer || v2.Num != 42 {
		t.Fatalf("second Read: v=%+v err=%v", v2, err)
	}
	v3, err := r.Read()
	if err != nil || v3.Type != BulkString || v3.Str != "foo" {
		t.Fatalf("third Read: v=%+v err=%v", v3, err)
	}
}

// --- Malformed input should error, not panic ---

func TestReadUnknownTypePrefix(t *testing.T) {
	r := NewReader(strings.NewReader("!oops\r\n"))
	_, err := r.Read()
	if err == nil {
		t.Fatalf("expected error for unknown type prefix, got nil")
	}
}

// --- Marshal round trips (Marshal is done for you — these confirm your
// Read output is shaped correctly by feeding it back through Marshal
// and re-parsing) ---

func TestRoundTripBulkString(t *testing.T) {
	original := BulkStringValue("hello world")
	wire := original.Marshal()

	r := NewReader(strings.NewReader(string(wire)))
	parsed, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if parsed.Str != original.Str {
		t.Fatalf("round trip: got %q, want %q", parsed.Str, original.Str)
	}
}

func TestRoundTripArray(t *testing.T) {
	original := ArrayValue([]Value{
		BulkStringValue("GET"),
		BulkStringValue("foo"),
	})
	wire := original.Marshal()

	r := NewReader(strings.NewReader(string(wire)))
	parsed, err := r.Read()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(parsed.Array) != 2 || parsed.Array[0].Str != "GET" || parsed.Array[1].Str != "foo" {
		t.Fatalf("round trip: got %+v", parsed.Array)
	}
}
