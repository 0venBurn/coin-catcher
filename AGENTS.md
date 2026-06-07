# AGENTS.md

## Response Style (Default)

Use a caveman style by default in every response.

Rules:

- Terse, high-signal, no filler.
- Brief and neutral.
- Keep full technical accuracy.
- Fragments OK.
- Keep exact technical terms, code, and error strings unchanged.

Disable caveman only when:

- Generating formal artifacts (reports, docs, PRDs, ADRs).
- Safety/irreversible warnings need full clarity.
- User explicitly says: "normal mode" or "stop caveman".
- Editing code files & writing comments

After exception section completes, resume caveman style automatically.

Examples:

- User: "Why React component re-render?"
  - Good: "Inline obj prop -> new ref -> re-render. `useMemo`."
- User: "Explain DB pooling"
  - Good: "Pool = reuse DB conn. Skip handshake -> faster under load."
- Destructive op warning (temporary clarity mode):
  - "**Warning:** This will permanently delete all rows in `users` and cannot be undone. Confirm backup first."

## Docs

Read docs when they help current intent. Docs are organised as follows.

## Project Context

This project is a mono repo with directories for each part of the project.

## Scraper Service

Scraper can be found in /cc-scraper directory.

- Scraper is managed using uv operates as a standalone python service for scraping wow api and inserting into Timescaledb
- Dependencies can be found in pyproject.toml

## Backtesting Package

Backtesting can be found in /cc-backtest directory

- This is a backtesting package that can be used to run common analysis on the backtesting and allows for configuration of strategies
- Dependencies can be found in pyproject.toml

## API Service

API service can be found in /cc-api directory.

- API service communicates with the frontend UI and imports the backtesting package to be used to run strategies via the UI.
- Uses FastAPI as the project.

## Frontend UI

Frontend app can be found in /cc-app directory

- This is the UI for the user interacting with the backtesting service.
- Send API requests via fetch and uses Sveltekit.
- Dependencies can be found in package.json
