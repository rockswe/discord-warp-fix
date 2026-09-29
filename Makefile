VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
LDFLAGS := -s -w -X main.version=$(VERSION)
TARGETS := darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 windows/amd64 windows/arm64

.PHONY: build test lint dist install clean

build:
	go build -ldflags "$(LDFLAGS)" -o dwf ./cmd/dwf

test:
	go test ./...

lint:
	@test -z "$$(gofmt -l .)" || { gofmt -l .; echo "run gofmt -w ."; exit 1; }
	@for os in darwin linux windows; do GOOS=$$os go vet ./... || exit 1; done
	shellcheck install.sh server/setup-server.sh

dist:
	rm -rf dist && mkdir -p dist
	@for t in $(TARGETS); do \
		os=$${t%/*}; arch=$${t#*/}; ext=; [ $$os = windows ] && ext=.exe; \
		echo "dist/dwf-$$os-$$arch$$ext"; \
		CGO_ENABLED=0 GOOS=$$os GOARCH=$$arch go build -trimpath -ldflags "$(LDFLAGS)" -o dist/dwf-$$os-$$arch$$ext ./cmd/dwf || exit 1; \
	done
	cd dist && shasum -a 256 dwf-* > checksums.txt

install:
	./install.sh

clean:
	rm -rf dist dwf
