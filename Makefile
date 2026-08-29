.PHONY: tidy build run test lint docker up down

tidy:
	go mod tidy

build:
	go build -o bin/wa-gateway ./cmd/wa-gateway

run:
	go run ./cmd/wa-gateway

test:
	go test ./...

lint:
	go vet ./...

docker:
	docker build -t wa-gateway:dev .

up:
	docker compose up -d --build

down:
	docker compose down
