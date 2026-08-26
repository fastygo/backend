APP_NAME ?= headless-backend
GO       ?= go
DIST     ?= dist

SEED ?= ../@GitCourse/cms/gitcourse.data-seed.json

.PHONY: build run test conformance vet lint vuln race live-sql live-sql-up live-sql-down verify-light verify token backup restore seed docker-build docker-test clean

build:
	mkdir -p $(DIST)
	$(GO) build -trimpath -o $(DIST)/$(APP_NAME) ./cmd/server
	$(GO) build -trimpath -o $(DIST)/headless-token ./cmd/headless-token
	$(GO) build -trimpath -o $(DIST)/headless-backup ./cmd/headless-backup
	$(GO) build -trimpath -o $(DIST)/headless-seed ./cmd/headless-seed

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

live-sql-up:
	docker compose -f docker-compose.live-sql.yml up -d --wait

live-sql-down:
	docker compose -f docker-compose.live-sql.yml down --remove-orphans

live-sql: live-sql-up
	TEST_POSTGRES_DSN='postgres://postgres@127.0.0.1:5432/headless?sslmode=disable' \
	TEST_MYSQL_DSN='root@tcp(127.0.0.1:3306)/headless?parseTime=true&charset=utf8mb4' \
	TEST_MARIADB_DSN='root@tcp(127.0.0.1:3307)/headless?parseTime=true&charset=utf8mb4' \
	$(GO) test -count=1 -run TestLiveSQLDialects -v ./internal/storage/sqlstore
	$(MAKE) live-sql-down

verify-light: test conformance vet lint vuln
	$(GO) mod verify
	$(GO) build ./cmd/server ./cmd/headless-token ./cmd/headless-backup ./cmd/headless-seed

seed:
	$(GO) run ./cmd/headless-seed -path "$(SEED)"

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
