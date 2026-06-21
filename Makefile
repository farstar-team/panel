VERSION ?= dev
LDFLAGS := -s -w -X main.version=$(VERSION)

.PHONY: test vet build clean

test:
	go test ./...

vet:
	go vet ./...

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="$(LDFLAGS)" -o bin/farstar ./cmd/farstar

clean:
	rm -rf bin dist
