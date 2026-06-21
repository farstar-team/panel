# Architecture

Farstar is distributed as one statically linked Go binary.

## Components

### Control panel

The HTTP server exposes the embedded RTL web interface and authenticated JSON
API. Authentication sessions are stored in SQLite. Every state-changing
request requires a session-bound CSRF token.

### Store

SQLite runs in WAL mode with foreign keys and a busy timeout. Tunnel secrets
are encrypted before persistence. The encryption key is stored separately in
`master.key`.

### Process manager

Each running tunnel has a dedicated child process. This provides:

- independent restart and failure isolation
- simple PID-based lifecycle management
- per-tunnel log files
- preservation of the web panel if a tunnel crashes

### Tunnel engine

The engine transports multiplexed streams over:

- authenticated raw TCP
- authenticated WebSocket over TLS

Public server listeners map to client-side services by a two-byte mapping
index. Traffic copying handles partial writes and TCP half-close behavior.

### Security boundaries

The systemd unit runs as the unprivileged `farstar` user. It receives only the
`CAP_NET_BIND_SERVICE` capability so listeners can bind privileged ports when
needed. The filesystem is read-only except for `/etc/farstar`.

## Data flow

```text
Public user
    |
    v
Server public listener
    |
    v
Multiplexed TCP/WSS session
    |
    v
Client tunnel process
    |
    v
Local service
```
