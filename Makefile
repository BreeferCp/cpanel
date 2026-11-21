.PHONY: all test vet web build clean
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)

all: test build

vet:
	go vet ./...

test: vet
	go test ./...
	cd web && npm run typecheck

web:
	cd web && npm ci && npm run build

# Release tarballs: a static binary, the built UI and the deploy assets.
# Run `make clean` first for a pristine tree; the build itself never deletes.
build: web
	@for arch in amd64 arm64; do \
	  d=dist/breefercp-$(VERSION)-linux-$$arch; \
	  mkdir -p $$d; \
	  CGO_ENABLED=0 GOOS=linux GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o $$d/breeferd ./cmd/breeferd; \
	  cp -r web/dist $$d/web; cp -r deploy $$d/deploy; cp LICENSE README.md $$d/; \
	  tar -C dist -czf $$d.tar.gz breefercp-$(VERSION)-linux-$$arch; \
	done

clean:
	rm -rf dist web/dist
