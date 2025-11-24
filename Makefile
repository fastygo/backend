APP_NAME ?= go-backend
GO       ?= go

.PHONY: build run test lint docs docker-build

build:
	$(GO) build ./...

run:
	$(GO) run ./cmd/server

test:
	$(GO) test ./...

lint:
	$(GO) vet ./... && $(GO) fmt ./...

docs:
	@echo "Generating API docs... (update when swag is wired)"

docker-build:
	docker build -t $(APP_NAME):latest .

