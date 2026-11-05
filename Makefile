.PHONY: build run test docker-up docker-down
build:
	mkdir -p dist
	CGO_ENABLED=1 go build -trimpath -o dist/linkdesk ./cmd
run:
	CGO_ENABLED=1 go run ./cmd
test:
	CGO_ENABLED=1 go test -race ./...
	go vet ./...
docker-up:
	docker compose up --build -d
docker-down:
	docker compose down
