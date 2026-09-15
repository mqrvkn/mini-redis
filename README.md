# mini-redis

A Redis-compatible in-memory data store in Go.

## Run

```
go run ./cmd/server
```

Server listens on `:6380` by default.

## Week 1 (done)

TCP server, thread-safe store, basic commands over a simple line protocol.
Try it with `nc localhost 6380`:

```
SET foo bar
GET foo
EXISTS foo
DEL foo
GET foo
```

## Week 2 (done)

`internal/resp/resp.go` implements the real RESP wire protocol: `Read()`
parses Simple Strings, Errors, Integers, Bulk Strings (including null and
embedded-CRLF payloads), and Arrays (including nested and null arrays).
`internal/server/server.go` now uses `resp.Reader`/`Value.Marshal()`
instead of the old newline-delimited text protocol, so real `redis-cli`
can connect directly:

```
go test ./internal/resp/...   # full parser test suite
go run ./cmd/server            # start the server
redis-cli -p 6380              # in another tab, if you have redis-cli installed
```

Then in the `redis-cli` session: `SET foo bar`, `GET foo`, `DEL foo`, etc.
work exactly like talking to real Redis.

If you don't have `redis-cli` installed: `brew install redis` on macOS
gets you the CLI tool without needing to run the actual Redis server.

## Project layout

```
cmd/server/main.go        — entry point, flag parsing, wiring
internal/store/store.go   — thread-safe map (the "database")
internal/server/server.go — TCP accept loop + per-connection handler + command dispatch
internal/resp/resp.go     — RESP wire protocol parser (Week 2)
internal/resp/resp_test.go — test suite for the parser
```
