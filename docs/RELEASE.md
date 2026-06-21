# Release process

1. Update `CHANGELOG.md`.
2. Run all checks:

```bash
gofmt -w .
go test -race ./...
go vet ./...
bash -n install.sh uninstall.sh
node --check internal/httpapi/web/app.js
```

3. Commit the release changes.
4. Create and push a semantic version tag:

```bash
git tag -a v0.1.0 -m "Farstar Tunnel Panel v0.1.0"
git push origin v0.1.0
```

The Release workflow builds Linux AMD64 and ARM64 binaries, creates
`SHA256SUMS`, and publishes all three files in a GitHub Release.

The installer automatically discovers the latest published release.
