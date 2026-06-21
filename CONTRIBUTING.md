# Contributing

## Development

Requirements:

- Go 1.24 or newer
- Node.js only for optional JavaScript syntax checks

Run:

```bash
go test ./...
go vet ./...
go build ./cmd/farstar
node --check internal/httpapi/web/app.js
```

## Pull requests

- Keep changes focused.
- Add tests for behavior changes.
- Do not commit runtime databases, master keys, logs, or credentials.
- Preserve RTL and mobile layouts when changing the web interface.
- Explain security implications for authentication, encryption, networking, or
  process-management changes.
