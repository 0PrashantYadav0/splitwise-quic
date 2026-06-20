# AGENTS.md — splitwise-quic

Shared-expense tracker built entirely over HTTP/3 + QUIC, with real-time updates
via WebTransport DATAGRAM frames. Go 1.26, pure-Go SQLite (no cgo).

---

## Build & Run

```bash
go build -o splitwise-quic .          # compile binary
go run .                              # run from source (default :4433)
go run . -addr :8443 -db dev.db       # custom port / db path
go run ./cmd/h3check https://localhost:4433/login  # HTTP/3 smoke test
```

Flags:
- `-addr` — listen address (default `:4433`)
- `-db`   — SQLite file path (default `splitwise.db`)
- `-uploads` — receipt upload directory (default `uploads`)

Environment:
- `REQUIRE_MTLS=1` — enable mutual TLS (requires client certificate)

> **QUIC needs UDP.** Port 4433 must be open on both TCP and UDP. TCP serves
> the first request + Alt-Svc upgrade header; QUIC takes over on subsequent
> requests.

---

## Testing

```bash
go test ./...     # run all tests
go vet ./...      # static analysis
```

Tests live in `internal/splits/compute_test.go`. They are pure unit tests —
no database, no mocks, no network. New split-math or debt-simplification logic
**must** have a corresponding test here.

---

## Project Layout

```
main.go                    # flag parsing, dependency wiring, graceful shutdown
cmd/h3check/main.go        # standalone HTTP/3 client (dev smoke test)
internal/
  models/models.go         # domain types (User, Group, Expense, Share, etc.)
  db/db.go                 # SQLite connection, schema, WAL config
  store/                   # persistence layer (auth, groups, expenses, balances, comments)
  splits/                  # pure money math — split algorithms + debt minimization
  server/                  # QUIC/HTTP3 server, TLS cert generation
  realtime/hub.go          # in-memory pub/sub (sync.RWMutex, buffered channels)
  render/render.go         # embedded HTML templates + static assets
  handlers/                # HTTP routing, middleware, request handlers
```

---

## Key Conventions

### Money — integers only
All amounts are stored and computed as **integer minor units** (cents).
- Parse at the input boundary: `"12.34"` → `1234`
- Render with the `Money(minor int64)` helper: `1234` → `"12.34"`
- **Never use `float64` for money.** The `splits` package guarantees zero
  penny loss across all split modes.

### Database migrations — additive only
Schema changes go in `internal/db/db.go` as `ALTER TABLE` statements.
The code intentionally ignores "duplicate column" errors so migrations are
idempotent. Never drop or rename columns — add new ones.

### Embedded assets
Templates and static files are compiled into the binary via `//go:embed`:
- `internal/render/templates/*.html`
- `internal/render/static/*`

There are no external asset files at runtime. Edit files under those paths;
the embed picks them up at next build.

### Error handling
- `ErrNotFound` — sentinel for missing rows (check with `errors.Is`)
- Handlers use `httpError(w, msg, code)` for early returns
- DB transactions always `defer tx.Rollback()` (idempotent after commit)

### Split types
Four modes in `internal/splits/compute.go`:
| Type | Notes |
|------|-------|
| `equal` | Remainder cents distributed to first N participants |
| `exact` | Caller-supplied amounts; must sum to total |
| `percentage` | Basis points (hundredths of a percent); must sum to 10,000 |
| `shares` | Proportional; rounding by largest fractional parts |

---

## QUIC / WebTransport Gotchas

### TLS cert is regenerated every boot
`internal/server/tls.go` generates a fresh ECDSA P-256 self-signed cert on
startup (valid 13 days). WebTransport requires:
- ECDSA key (RSA won't work)
- Validity ≤ 14 days
- SHA-256 hash passed to the browser's `serverCertificateHashes` option

The hash is served at `/cert-hash` and injected into the HTMX page. Do not
cache or hard-code this value.

### WebTransport endpoints
- `GET /g/{id}/wt` — group-level datagram channel (all group members)
- `GET /wt` — per-user personal push channel

Both send fire-and-forget DATAGRAM frames (`session.SendDatagram`). Loss is
acceptable; do not use for anything requiring delivery guarantees.

### Alt-Svc upgrade
Every TCP response carries `Alt-Svc: h3=":4433"; ma=2592000`. The browser
upgrades automatically on the next request. First-load is always TCP.

---

## Dependencies (key)

| Package | Purpose |
|---------|---------|
| `github.com/quic-go/quic-go` | QUIC transport |
| `github.com/quic-go/webtransport-go` | WebTransport over QUIC |
| `modernc.org/sqlite` | Pure-Go SQLite (no cgo required) |
| `github.com/go-pdf/fpdf` | PDF report generation (`handlers/export.go`) |
| `github.com/google/uuid` | Session tokens |
| `golang.org/x/crypto` | bcrypt for password hashing |
