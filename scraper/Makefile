.PHONY: format test run up down logs db

format:
	cd .. && gofmt -w cmd scraper

test:
	cd .. && go test ./...

run:
	cd .. && go run ./cmd/scraper

up:
	cd .. && docker compose up --build -d

down:
	cd .. && docker compose down

logs:
	cd .. && docker compose logs -f scraper db

db:
	cd .. && docker compose exec db psql -U coin_catcher -d coin_catcher
