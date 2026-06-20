<div align="center">

#  Splitwise-QUIC

### A Splitwise clone that talks to your browser **entirely over HTTP/3 + QUIC**

Shared-expense tracking with real-time updates delivered as **QUIC DATAGRAM frames** over WebTransport.

<br/>

![Go](https://img.shields.io/badge/Go-1.26-00ADD8?logo=go&logoColor=white)
![HTTP/3](https://img.shields.io/badge/HTTP%2F3-QUIC-ff6c37)
![WebTransport](https://img.shields.io/badge/WebTransport-DATAGRAMs-8e44ad)
![SQLite](https://img.shields.io/badge/SQLite-pure--Go-003B57?logo=sqlite&logoColor=white)
![HTMX](https://img.shields.io/badge/HTMX-1.9-3366cc)
![Tailwind](https://img.shields.io/badge/Tailwind-CDN-38bdf8?logo=tailwindcss&logoColor=white)
![License](https://img.shields.io/badge/license-MIT-green)

</div>

---

##  Table of contents

- [Splitwise-QUIC](#splitwise-quic)
    - [A Splitwise clone that talks to your browser **entirely over HTTP/3 + QUIC**](#a-splitwise-clone-that-talks-to-your-browser-entirely-over-http3--quic)
  - [Table of contents](#table-of-contents)
  - [What is this?](#what-is-this)
  - [Highlights](#highlights)
  - [Architecture](#architecture)
  - [The QUIC techniques](#the-quic-techniques)
  - [Splitwise features](#splitwise-features)
  - [Quick start](#quick-start)
    - [Run with Docker](#run-with-docker)
    - [Deploy to Kubernetes](#deploy-to-kubernetes)
  - [Verifying HTTP/3](#verifying-http3)
  - [Project layout](#project-layout)
  - [How it works](#how-it-works)
    - [The TCP -\> QUIC upgrade dance](#the-tcp---quic-upgrade-dance)
    - [Real-time updates (two channels)](#real-time-updates-two-channels)
    - [Money, the right way](#money-the-right-way)
    - [Debt simplification](#debt-simplification)
  - [Configuration](#configuration)
  - [Testing](#testing)
  - [Design decisions](#design-decisions)
  - [Troubleshooting](#troubleshooting)
  - [Roadmap](#roadmap)

---

##  What is this?

**Splitwise-QUIC** is a fully working [Splitwise](https://www.splitwise.com/)-style
expense splitter, built as a deliberate playground for the *complex* QUIC
techniques you rarely see wired together in one real application.

Your browser bootstraps over **HTTP/2 (TCP/TLS)**, reads the `Alt-Svc` header,
and transparently upgrades to **HTTP/3 over QUIC (UDP)** for every subsequent
request — both served on the same port. On top of that, live balance updates are
pushed to the browser as unreliable **QUIC DATAGRAM frames** via WebTransport,
with a graceful Server-Sent-Events fallback for non-Chromium browsers.

It's a real app (groups, multi-currency expenses, debt simplification, settle-up)
that happens to be an end-to-end QUIC showcase.

---

##  Highlights

| | |
|---|---|
|  **HTTP/3 first** | Every page and partial is served over QUIC; TCP exists only to bootstrap the upgrade. |
|  **Live over datagrams** | Real-time UI updates via QUIC DATAGRAMs (WebTransport) + SSE fallback. |
|  **Correct money math** | Integer minor units everywhere; zero lost pennies across any split. |
|  **4 split modes** | Equal, exact, percentage, and weighted shares. |
|  **Multi-currency** | Each currency's balances tracked and simplified independently. |
|  **Debt simplification** | Max-heap greedy cash-flow minimization — O(n log n), ≤ n+m-1 transfers guaranteed. |
|  **Self-contained binary** | Templates + static assets embedded via `go:embed`; pure-Go SQLite (no cgo). |
|  **Zero-config TLS** | Fresh short-lived ECDSA cert minted on every boot for WebTransport cert-hash pinning. |

---

##  Architecture

> An interactive, zoomable version with multiple diagrams (system, request
> lifecycle, data model, QUIC handshake) lives in
> **[`docs/architecture.html`](docs/architecture.html)** — just open it in a browser.

```mermaid
flowchart TB
    subgraph Client["Browser"]
        UI["HTMX + Tailwind UI"]
        JS["app.js<br/>protocol badge / split UX"]
        WTC["WebTransport client<br/>(QUIC datagrams)"]
    end

    subgraph Server["Go server :4433"]
        direction TB
        TCP["TCP listener<br/>HTTP/1.1 + HTTP/2"]
        UDP["UDP listener<br/>HTTP/3 + QUIC"]
        ALT["Alt-Svc middleware<br/>(advertise h3)"]
        MUX["http.ServeMux<br/>+ auth middleware"]
        H["Handlers<br/>pages / actions / partials"]
        RT["realtime.Hub<br/>(pub/sub)"]
        ST["store<br/>(persistence)"]
        SP["splits<br/>(money math + debt)"]
    end

    DB[("SQLite<br/>WAL")]

    UI -->|"HTTP/2 bootstrap"| TCP
    UI -->|"HTTP/3 (after Alt-Svc)"| UDP
    WTC -->|"QUIC DATAGRAMs"| UDP
    TCP --> ALT
    UDP --> ALT
    ALT --> MUX --> H
    H --> ST --> DB
    H --> SP
    H -->|publish| RT
    RT -->|"SSE + datagrams"| UI
    RT -.->|datagrams| WTC
```

---

##  The QUIC techniques

This project intentionally stacks the "hard" parts of QUIC into one app:

| Technique | Where it lives |
|---|---|
| **HTTP/3 over QUIC** (TLS 1.3 mandatory) | `internal/server/server.go` |
| **0-RTT** session resumption | `quic.Config{ Allow0RTT: true }` |
| **QUIC DATAGRAMs** (RFC 9221) | `EnableDatagrams: true` + WebTransport push |
| **WebTransport** live channel (browser) | `internal/handlers/realtime.go`, `static/app.js` |
| **Per-user push** over datagrams | topic-based `realtime.Hub` + `/wt` endpoint |
| Receipt upload **over a QUIC stream** | multipart body on HTTP/3 = a dedicated stream |
| **Stream multiplexing** tuned high | `MaxIncomingStreams: 512` (no head-of-line blocking) |
| **Connection migration** friendliness | keep-alive + QUIC path validation |
| **Alt-Svc** TCP -> QUIC upgrade hint | `withAltSvc` middleware |
| **Mutual TLS** (optional) | `REQUIRE_MTLS=1` |
| Short-lived **ECDSA cert** for cert-hash pinning | `internal/server/tls.go` |

---

##  Splitwise features

- **Auth** — email/password with bcrypt hashing + opaque session cookies
- **Groups** — create groups, add members
- **Expenses** with four split modes:
  - **Equal** — divided evenly, leftover cents distributed deterministically
  - **Exact** — explicit amounts that must reconcile to the total
  - **Percentage** — basis-point precision, must total 100%
  - **Shares** — weighted (e.g. 2:1 => two-thirds / one-third)
- **Multi-currency** — balances computed per currency, never mixed
- **Debt simplification** — minimizes the number of "who pays whom" transfers
- **Settle-up** — record direct payments that clear balances
- **Expense editing** — edit any expense in place (htmx-swapped form, re-computes shares)
- **Comments** — threaded notes on each expense
- **Receipt uploads** — attach a photo to an expense (streamed over QUIC)
- **CSV / PDF export** — download a group's expenses (CSV) or a full report (PDF)
- **Activity feed** — human-readable audit trail per group
- **Real-time** — instant UI refresh via QUIC datagrams (WebTransport) or SSE
- **Per-user push notifications** — personal alerts delivered as QUIC datagrams
  on *any* page (added to a group, a new expense, a settlement)
- **Dark / light mode** — starfield theme with a persistent toggle

---

##  Quick start

**Prerequisites:** Go 1.26+

```bash
# from the project root
go run .
```

Then open **<https://localhost:4433>**.

> The dev server uses a **self-signed certificate**, so your browser will warn
> once. Accept it to proceed (a fresh cert is minted on every boot).

Build a binary instead:

```bash
go build -o splitwise-quic .
./splitwise-quic -addr :4433 -db splitwise.db -uploads uploads
```

### Run with Docker

```bash
docker compose up --build
# or, on a restricted module-proxy network:
# docker build --build-arg GOPROXY=direct -t splitwise-quic .
```

Then open **<https://localhost:4433>**. The SQLite DB and uploaded receipts
persist in the `sqquic-data` volume.

### Deploy to Kubernetes

```bash
kubectl apply -f deploy/k8s.yaml
```

Runs as a single replica (SQLite is single-writer) with a `ReadWriteOnce` PVC
for `/data`. Liveness/readiness probes hit `/healthz`. Note that QUIC needs the
UDP port exposed alongside TCP — a mixed-protocol LoadBalancer (k8s 1.26+) or
two Services.

---

##  Verifying HTTP/3

Most system `curl` builds **don't** ship HTTP/3 support, so a tiny QUIC client is
bundled:

```bash
go run ./cmd/h3check https://localhost:4433/login
# -> OK over HTTP/3.0 -> 200 OK (3211 bytes)
```

Check the Alt-Svc upgrade hint over plain TCP:

```bash
curl -sk -D - -o /dev/null https://localhost:4433/login | grep -i alt-svc
# -> alt-svc: h3=":4433"; ma=2592000
```

In the browser, the green **`proto: h3`** badge in the header confirms you're on
HTTP/3, and the pulsing **live** dot on a group page confirms the WebTransport
datagram channel is connected.

---

##  Project layout

```
splitwise-quic/
├── main.go                  # entry point: flags, wiring, graceful shutdown
├── cmd/
│   └── h3check/             # standalone HTTP/3 client (smoke test)
├── internal/
│   ├── models/              # domain types (User, Group, Expense, ...)
│   ├── db/                  # SQLite connection + schema migration
│   ├── store/               # persistence (users, groups, expenses, balances)
│   ├── splits/              # PURE money math: split modes + debt simplification
│   ├── server/              # QUIC/HTTP3 transport, TLS, Alt-Svc, listeners
│   ├── realtime/            # in-memory pub/sub hub
│   ├── render/              # embedded templates (HTMX) + static assets (JS)
│   └── handlers/            # HTTP handlers, routing, SSE, WebTransport,
│                            #   edit/comments/receipts/export
├── deploy/
│   └── k8s.yaml             # Kubernetes Deployment + Service + PVC
├── docs/
│   └── architecture.html    # interactive Mermaid architecture diagrams
├── Dockerfile               # multi-stage, distroless, non-root
├── docker-compose.yml
└── README.md
```

Every file is comfortably under 600 lines, and the `splits` package is **pure**
(no I/O) so the tricky money logic is trivially testable.

---

##  How it works

### The TCP -> QUIC upgrade dance

1. Browser makes its first request over **HTTP/2 (TCP/TLS)**.
2. Server responds with an **`Alt-Svc: h3=":4433"`** header.
3. Browser remembers this and uses **HTTP/3 over QUIC (UDP)** for subsequent
   requests — same port, different transport.

### Real-time updates (two channels)

- **WebTransport (fast lane):** `app.js` opens a WebTransport session pinned to
  the server's SHA-256 cert hash and reads **QUIC DATAGRAM** frames. Each event
  triggers an HTMX partial refresh.
- **SSE (fallback):** the page also subscribes via the HTMX SSE extension, so
  browsers without WebTransport still get live updates.

Both are fed by the same `realtime.Hub` — handlers `Publish` an event after any
mutation, and every subscriber (SSE stream or WT session) fans it out.

### Money, the right way

All amounts are stored as **integer minor units** (cents). Floating point only
appears at the input-parsing boundary. Splits distribute any rounding remainder
one cent at a time to the largest fractional parts — so shares **always** sum to
the exact total.

### Debt simplification

Net balances per currency feed a **max-heap greedy cash-flow minimization** algorithm
(`internal/splits/simplify.go`):

1. Build two max-heaps — one for creditors (net > 0), one for debtors (net < 0).
2. At each step, pop the **largest creditor** and **largest debtor**.
3. Settle `min(debtor, creditor)` between them, then re-insert whichever side has a remainder.
4. Repeat until both heaps are empty.

This guarantees at most **n + m − 1 transfers** (the theoretical minimum for n debtors and
m creditors), while always re-evaluating the largest remaining obligation after every partial
payment — something a sort-once two-pointer scan cannot do. Time: **O(n log n)**. Space: **O(n)**.

**Error handling:** zero-balance entries are skipped; an imbalanced ledger (sum ≠ 0) is logged
but does not abort — the algorithm clears as much debt as possible.

### Split error handling

`splits.Compute` returns typed sentinel errors (use `errors.Is`):

| Error | Trigger |
|---|---|
| `ErrNonPositiveTotal` | Total ≤ 0 |
| `ErrEmptyInputs` | No participants |
| `ErrDuplicateUser` | Same UserID appears twice |
| `ErrNegativeValue` | Negative amount/weight/percentage |
| `ErrOverflow` | `total × share-weight` exceeds int64 |
| `ErrBadSplit` | Exact amounts or percentages don't reconcile |
| `ErrInvalidSplitType` | Unknown split type string |

---

##  Configuration

| Flag / Env | Default | Description |
|---|---|---|
| `-addr` | `:4433` | Listen address (used for **both** TCP and UDP) |
| `-db` | `splitwise.db` | SQLite database file path |
| `-uploads` | `uploads` | Directory for uploaded receipt images |
| `REQUIRE_MTLS` | _unset_ | Set to `1` to require mutual TLS (clients must present a cert) |

---

##  Testing

```bash
go test ./...        # split math + debt-simplification correctness
go vet ./...         # static analysis
```

The test suite covers: guard conditions (zero/negative total, empty inputs, duplicate user,
negative values, overflow), equal-split penny distribution, exact-split reconciliation,
percentage basis points, weighted shares, heap reorder after partial payment, and
minimal-transfer guarantees for multi-creditor/debtor groups.

---

##  Design decisions

- **Pure-Go SQLite** (`modernc.org/sqlite`) — no cgo, so the build stays simple
  and cross-compilable. WAL mode + busy timeout keep concurrent QUIC streams from
  tripping over locks.
- **`go:embed` everything** — templates and static assets ship inside the binary;
  deploy a single file.
- **Short-lived ECDSA cert** — WebTransport's `serverCertificateHashes` only
  accepts ECDSA certs valid for <= 14 days, so a fresh 13-day cert is generated on
  every startup. No CA to install.
- **Manual `Alt-Svc` header** — set directly in middleware to avoid a
  listener-registration race in `quic-go`'s `SetQUICHeaders`.
- **Integer money** — floats are banned past the input boundary.

---

##  Troubleshooting

| Symptom | Cause / fix |
|---|---|
| Browser shows a cert warning | Expected — self-signed dev cert. Accept it once. |
| `proto: h2` instead of `h3` | First load is always HTTP/2; reload after the Alt-Svc header lands. |
| Live dot says "SSE fallback" | Your browser lacks WebTransport (Firefox/Safari). Updates still work via SSE. |
| `curl: option --http3 ...not support` | System curl has no HTTP/3. Use `go run ./cmd/h3check` instead. |
| Port already in use | Another instance is running: `pkill -f 'splitwise-quic -addr'`. |

---

##  Roadmap

- [x] Expense editing & comments
- [x] Receipt photo uploads (over QUIC streams)
- [x] Per-user push notifications via datagrams
- [x] CSV / PDF export
- [x] Dockerfile + deployment manifest

---

<div align="center">

Built over QUIC, one penny at a time. 

</div>
