# Coin Catcher

A **monorepo** for a **World of Warcraft (WoW) commodities market** data warehousing and backtesting solution. Scrapes auction house data, stores it in a time-series database, and provides tools for backtesting trading strategies.

> Built for learning: Python, async programming, data scraping, warehousing, and full-stack product development.

---

## 📁 Project Structure

| Directory  | Description                          | Tech Stack                     |
|------------|--------------------------------------|--------------------------------|
| `/scraper` | Scrapes WoW API → TimescaleDB/Postgres | Python 3.13+, `uv`, `httpx`, `psycopg` |
| `/backtest`| Backtesting engine for strategies    | Python 3.13+                   |
| `/api`     | REST API for frontend                | Python 3.13+, FastAPI (planned) |
| `/app`     | Frontend UI                          | SvelteKit, TypeScript, pnpm    |

---

## ⚙️ Setup

### Prerequisites
- **Python 3.13+** (for `/scraper`, `/backtest`, `/api`)
- **Node.js 20+** (for `/app`)
- **pnpm** (recommended for `/app`)
- **PostgreSQL/TimescaleDB** (for data storage)
- **Blizzard API credentials** (for scraping)

---

### 1. Clone & Install

```bash
# Clone the repo
git clone https://github.com/0venBurn/coin-catcher.git
cd coin-catcher

# Install Python dependencies for each service
cd scraper && uv sync
cd ../backtest && uv sync
cd ../api && uv sync

# Install frontend dependencies
cd app && pnpm install
```

---

### 2. Configure Environment

#### Scraper (`/scraper`)
Create a `.env` file in `/scraper`:
```env
CLIENT_ID=your_blizzard_client_id
CLIENT_SECRET=your_blizzard_client_secret
DATABASE_URL=postgres://user:password@localhost:5432/coin_catcher
```

> **Note:** Blizzard API credentials are required for scraping. Register an app at [Blizzard Developer Portal](https://develop.battle.net/).

#### Frontend (`/app`)
Copy `.env.example` to `.env` and update:
```env
DATABASE_URL=postgres://user:password@localhost:5432/coin_catcher
BETTER_AUTH_SECRET=your_high_entropy_secret
ORIGIN=http://localhost:5173
```

---

### 3. Database Setup

1. **Create a PostgreSQL database** (TimescaleDB recommended for time-series data):
   ```bash
   createdb coin_catcher
   ```

2. **Run migrations** (from `/app`):
   ```bash
   pnpm db:push
   ```

---

## 🚀 Running the Project

### Scraper
```bash
cd scraper
uv run python main.py
```
> **Note:** The scraper is currently a skeleton. You'll need to implement the full WoW API scraping logic (see `/scraper/async_client.py` for a starting point).

### Backtest
```bash
cd backtest
uv run python main.py
```
> **Note:** The backtest module is a placeholder. Implement your trading strategies here.

### API
```bash
cd api
uv run python main.py
```
> **Note:** The API is a stub. It will eventually use FastAPI to expose backtest results to the frontend.

### Frontend
```bash
cd app
pnpm dev
```
> Opens at `http://localhost:5173` (or `3000` in production).

---

## 📦 Service Details

### Scraper
- **Purpose:** Fetches WoW auction house data via Blizzard API.
- **Key Files:**
  - `main.py`: Entry point (stub).
  - `async_client.py`: Async HTTP client for Blizzard API (includes OAuth token fetching).
- **Dependencies:** `httpx`, `psycopg[binary]`.

### Backtest
- **Purpose:** Runs trading strategies against historical data.
- **Current State:** Placeholder (`main.py` prints a greeting).

### API
- **Purpose:** Serves backtest results to the frontend.
- **Current State:** Placeholder (`main.py` prints a greeting).
- **Planned:** FastAPI-based REST endpoints.

### Frontend
- **Framework:** SvelteKit (v2) + TypeScript.
- **Features:**
  - Authentication via [Better Auth](https://better-auth.com).
  - Database ORM: [Drizzle](https://orm.drizzle.team).
  - Testing: Vitest (unit), Playwright (E2E).
- **Scripts:**
  - `pnpm dev`: Start dev server.
  - `pnpm build`: Production build.
  - `pnpm db:push`: Sync database schema.

---

## 🔧 Development Notes

- **Python:** Uses [`uv`](https://github.com/astral-sh/uv) for dependency management (faster than `pip`).
- **Frontend:** Uses `pnpm` for efficiency (workspace-aware).
- **Database:** Designed for TimescaleDB (PostgreSQL extension for time-series data).

---

## ⚠️ Important Notes

1. **Hosted Version:** The project may be sunset. The hosted version (if any) could be removed.
2. **Data Collection:** Meaningful backtests require **weeks of historical data**. Plan for sustained scraping.
3. **Blizzard API Limits:** Respect rate limits and terms of service.
4. **Self-Hosting:** To use this project, you must:
   - Implement the scraper logic.
   - Set up your own database.
   - Configure Blizzard API credentials.

---

## 📜 License

MIT (implied). Use at your own risk.
