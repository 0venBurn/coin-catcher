# AGENTS.md

## Docs

Read docs when they help current intent. Docs are organised as follows.

/docs/code -> contains knowledge around system patterns, conventions and testing.

- system_patterns.md shows architectural overviews on what happens in each service.
- conventions.md considers the conventions for a project
- testing.md considers the testing strategy for the variety of changes/ implementation

/docs/adrs -> contain architectural design records for decisions made in the process

/docs/reports -> contains specific html reports generated about research.

/docs/prds -> contains product requirement docs around specific issues. Can be used in code reviews or when implementing.

## Project Context

This project is a mono repo with directories for each part of the project.

## Scraper Service

Scraper can be found in /scraper directory.

- Scraper is managed using uv operates as a standalone python service for scraping wow api and inserting into Timescaledb
- Dependencies can be found in pyproject.toml

## Backtesting Package

Backtesting can be found in /backtest directory

- This is a backtesting package that can be used to run common analysis on the backtesting and allows for configuration of strategies
- Dependencies can be found in pyproject.toml

## API Service

API service can be found in /api directory.

- API service communicates with the frontend UI and imports the backtesting package to be used to run strategies via the UI.
- Uses FastAPI as the project.

## Frontend UI

Frontend app can be found in /app directory

- This is the UI for the user interacting with the backtesting service.
- Send API requests via fetch and uses Sveltekit.
- Dependencies can be found in package.json
