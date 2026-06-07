.PHONY: lint format typecheck all install run

lint:
	ruff check src/

format:
	ruff format src/

typecheck:
	basedpyright src/

install:
	uv sync

run:
	uv run scraper

all: lint format typecheck
