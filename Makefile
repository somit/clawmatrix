VERSION ?= $(shell git describe --tags --always --dirty)
DIST    := dist

.PHONY: build build-clutch build-control-plane build-llm-proxy templ clean test

build: build-clutch build-control-plane build-llm-proxy

templ:
	templ generate ./control-plane/internal/ui/

build-clutch:
	cd clutch && CGO_ENABLED=0 go build -ldflags "-s -w -X main.gatewayVersion=$(VERSION)" -o ../$(DIST)/clutch .

build-control-plane: templ
	cd control-plane && go build -ldflags "-s -w -X main.version=$(VERSION)" -o ../$(DIST)/control-plane .

build-llm-proxy:
	cd llm-proxy && CGO_ENABLED=0 go build -ldflags "-s -w" -o ../$(DIST)/llm-proxy .

test:
	cd clutch && go test ./...
	cd control-plane && go test ./...

clean:
	rm -rf $(DIST)/
