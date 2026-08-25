APP_NAME ?= headless-backend
GO       ?= go
DIST     ?= dist

.PHONY: build run test conformance vet lint vuln race live-sql verify token backup restore docker-build docker-test clean

build:
	mkdir -p $(DIST)
	$(GO) build -trimpath -o $(DIST)/$(APP_NAME) ./cmd/server
	$(GO) build -trimpath -o $(DIST)/headless-token ./cmd/headless-token
	$(GO) build -trimpath -o $(DIST)/headless-backup ./cmd/headless-backup

run:
	$(GO) run ./cmd/server

test:
	$(GO) test -count=1 ./...

conformance:
	$(GO) test -count=1 ./internal/conformance

vet:
	$(GO) vet ./...

lint:
	$(GO) tool staticcheck ./...

vuln:
	$(GO) tool govulncheck ./...

ifeq ($(OS),Windows_NT)
race:
	docker compose -f docker-compose.test.yml run --build --rm test
else
race:
	$(GO) test -race -count=1 -p 2 ./...
endif

live-sql:
	$(GO) test -count=1 -run TestLiveSQLDialects -v ./internal/storage/sqlstore

verify-light: test conformance vet lint vuln
	$(GO) mod verify
	$(GO) build ./cmd/server ./cmd/headless-token ./cmd/headless-backup

verify: verify-light race

token:
	$(GO) run ./cmd/headless-token -subject "$(SUBJECT)" -role "$(ROLE)" -ttl "$(TTL)"

backup:
	$(GO) run ./cmd/headless-backup -mode export -path "$(PATH)"

restore:
	$(GO) run ./cmd/headless-backup -mode restore -path "$(PATH)"

docker-build:
	docker build -t $(APP_NAME):latest .

docker-test:
	docker compose -f docker-compose.test.yml up --build --abort-on-container-exit

clean:
	$(GO) clean
	rm -rf "$(DIST)"
