.PHONY: run test tidy

run: 
	go run ./cmd/scraper

test:
	go test ./...

tidy: 
	go mod tidy

build:
	go build -o bin/scraper ./cmd/scraper


