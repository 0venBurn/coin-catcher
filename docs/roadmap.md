# Roadmap

Work is tracked in Linear. This file is a snapshot of the 2026-09-04 plan so the repo points at the same sequence.

- Product project: [CoinCatcher](https://linear.app/evanbyrnecodingprojects/project/coincatcher-c2240ff3c0e8)
- Platform project: [Homelab](https://linear.app/evanbyrnecodingprojects/project/homelab-e582c07364dc)
- Product plan: [warehouse, rewrite, backtest, UI](https://linear.app/evanbyrnecodingprojects/document/plan-warehouse-rewrite-backtest-ui-67fab95068bb)
- Platform plan: [storage, tunnel, k3s, GitOps](https://linear.app/evanbyrnecodingprojects/document/plan-storage-tunnel-k3s-gitops-5b2a40872c80)

## Current state

The Go scraper on `main` is agent-written and already deployed as Docker Compose (`db` + `scraper`). Treat it as a running data collector and a reference implementation, not as the long-term codebase.

Product shape:

1. Scraper (Blizzard commodities + WoW Token + TSM → TimescaleDB)
2. Backtest engine
3. Web UI (Go + HTMX) to browse markets and run strategies

## Sequence

Do Homelab storage and Cloudflare Tunnel SSH before the Go rewrite. Keep the live container running until the rewrite is proven.

```text
Homelab                         CoinCatcher
-------                         -----------
1 Durable storage    ---------> 1 Live warehouse
2 Cloudflare Tunnel SSH
3 k3s
4 GitOps
5 First tenant (current app)
                                2 Rewrite scraper in Go (learning path)
                                3 Backtest
                                4 Strategy UI (Go + HTMX)
                                5 GitOps cutover of the rewrite
```

Tickets are **vertical slices**: each one demos an outcome and the next slice adds a capability. Layered work (config-only, httptest-only, “write tests later”) was folded in.

Pickup now:

- [RAN-146](https://linear.app/evanbyrnecodingprojects/issue/RAN-146) — TimescaleDB on a durable path, survives reboot
- [RAN-153](https://linear.app/evanbyrnecodingprojects/issue/RAN-153) — live scraper healthy + baseline

Then restore drill (`RAN-147`), then you-can-SSH (`RAN-149`) and friend SSH (`RAN-152`). Rewrite slices on `RAN-141` stay blocked until storage and SSH are done. The rewrite path is:

1. Process starts → 2. Print live token prices → 3. Persist/poll tokens → 4. EU auctions → 5. US + limiter → 6. TSM → 7. Item names → 8. Craft catalog → 9. Docker soak

## Guardrails

- Do not `docker compose down -v` on the live warehouse.
- Do not publish SSH, Postgres, or the Kubernetes API.
- Do not edit `internal/scraper/` in place for the rewrite; new `cmd/` + `internal/` tree.
- Copy invariants from `docs/data-invariants.md`, not files from the agent-written package.
