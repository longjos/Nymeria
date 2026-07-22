.PHONY: build run dev clean docker frontend backend windows linux-arm64 desktop-windows desktop-syso

VERSION ?= dev
LDFLAGS := -ldflags "-s -w -X main.version=$(VERSION)"
DESKTOP_LDFLAGS := -ldflags "-s -w -H=windowsgui -X main.version=$(VERSION)"

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
	go run $(LDFLAGS) ./cmd/nymeria --listen :9090

windows: frontend
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o nymeria.exe ./cmd/nymeria

linux-arm64: frontend
	GOOS=linux GOARCH=arm64 CGO_ENABLED=0 go build $(LDFLAGS) -o nymeria-linux-arm64 ./cmd/nymeria

# The committed cmd/nymeria-desktop/rsrc_windows_amd64.syso embeds the app
# icon + GUI manifest; `go build` links *_windows_amd64.syso automatically.
desktop-windows: frontend
	GOOS=windows GOARCH=amd64 go build $(DESKTOP_LDFLAGS) -o Nymeria-desktop-windows-amd64.exe ./cmd/nymeria-desktop

# Regenerate the Windows resource object from build/appicon.png (run after
# changing the icon, then commit the updated .syso).
desktop-syso:
	go run github.com/tc-hib/go-winres@v0.3.3 simply \
		--arch amd64 \
		--icon build/appicon.png \
		--manifest gui \
		--product-name Nymeria \
		--file-description "Nymeria APRS desktop client" \
		--original-filename Nymeria-desktop-windows-amd64.exe \
		--product-version 0.0.0 \
		--file-version 0.0.0 \
		--out cmd/nymeria-desktop/rsrc

clean:
	rm -f nymeria nymeria.exe nymeria-linux-arm64 Nymeria-desktop-windows-amd64.exe
	rm -rf web/build web/.svelte-kit web/node_modules

docker:
	docker build -t nymeria:$(VERSION) .

test:
	go test ./...

lint:
	go vet ./...

.DEFAULT_GOAL := build
