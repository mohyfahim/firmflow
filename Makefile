APP_NAME := firmflow-api

.PHONY: setup up down run test lint tidy fmt migrate

setup:
	cp -n .env.example .env || true
	go mod tidy

up:
	docker compose up -d

down:
	docker compose down

build:
	docker compose build app

run:
	go run ./cmd/server

test:
	go test ./...

lint:
	go vet ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

migrate:
	DB_AUTO_MIGRATE=true go run ./cmd/server
