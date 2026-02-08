.PHONY: build run dev clean docker frontend backend

VERSION ?= dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"

build: frontend backend

frontend:
	cd web && pnpm install && pnpm build

backend:
	go build $(LDFLAGS) -o nymeria ./cmd/nymeria

run: build
	./nymeria

dev:
	@echo "Starting frontend dev server..."
	cd web && pnpm dev &
	@echo "Starting Go server..."
	go run $(LDFLAGS) ./cmd/nymeria --dev

clean:
	rm -f nymeria
	rm -rf web/build web/.svelte-kit web/node_modules

docker:
	docker build -t nymeria:$(VERSION) .

test:
	go test ./...

lint:
	go vet ./...

.DEFAULT_GOAL := build
