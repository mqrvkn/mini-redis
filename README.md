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

## Week 2 (in progress)

`internal/resp/resp.go` has a skeleton for the real RESP wire protocol —
`Read()` and its helpers are stubbed with TODOs. `internal/resp/resp_test.go`
has the full test suite; get it passing:

```
go test ./internal/resp/...
```

Suggested order to implement in `resp.go`:
1. `readLine` — the primitive everything else (except bulk string bodies) uses
2. `readSimpleString` and `readError` — simplest cases, sanity-check readLine
3. `readInteger`
4. `readBulkString` — the important one; pay attention to the length-prefix
   note in the comment
5. `readArray` — depends on all of the above
6. `Read` — dispatch on the type-prefix byte to the right helper above

Run `go test ./internal/resp/... -v` to see which specific case fails as you
go — the tests are ordered to match the implementation order above.

Once all tests pass, the next step (not yet wired up) is swapping
`server.dispatch`'s `strings.Fields` line parsing for `resp.Reader`, so
`redis-cli -p 6380` can talk to the server directly.

## Project layout

```
cmd/server/main.go        — entry point, flag parsing, wiring
internal/store/store.go   — thread-safe map (the "database")
internal/server/server.go — TCP accept loop + per-connection handler + command dispatch
internal/resp/resp.go     — RESP wire protocol parser (Week 2)
internal/resp/resp_test.go — test suite for the parser
```
