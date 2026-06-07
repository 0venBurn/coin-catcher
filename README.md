# CoinCatcher

This is a mono repo for the Coin Catcher Project. Data warehousing and backtesting solution for the World of Warcraft commodidities market.

## Layout

| Directory | Purpose |
|-----------|---------|
| `cc-scraper` | Blizzard API scraper (Python / uv) |
| `cc-backtest` | Backtesting strategies and analysis |
| `cc-api` | FastAPI backend for the UI |
| `cc-app` | SvelteKit frontend |

It was made for fun and to get better at python programming and explore data scraping, warehousing, backtesting & product building.

It can be run 100% locally or can be accessed at [Coin Catcher](https://localhost:3000) currently.

Project may be sunset removed from at some point so I'm leaving the ability for people to inspect the code and use it for their own configuration if needed.

But obviously the data scraping will need to be redone and may be a few weeks of uptime to be able to backtest.
