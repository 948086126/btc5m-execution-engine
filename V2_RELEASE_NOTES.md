# V2 Real No-Order Connectors

V2 adds Internet-facing market-data connectors only. It still contains no wallet, private key, signing, order placement, cancel, merge, or capital path.

## Added

- TLS RFC6455 client for `wss://` market-data streams.
- Binance USD-M BTCUSDT perpetual `bookTicker` parser and source runner.
- Binance Spot BTCUSDT `bookTicker` parser and source runner.
- Polymarket Market Channel subscription for two token IDs.
- Embedded BBO extraction from `price_change` / `best_bid_ask` events.
- Legal PM empty-side semantics remain:
  - `best_bid == 0` -> null bid
  - `best_ask == 1` -> null ask
- Array/non-BBO PM frames are tolerated without being misclassified as BBO failures.
- Receive wall-clock, monotonic time, source receive sequence, and raw frame SHA are captured before parsing.
- One normalized PM BBO state is emitted per received PM frame after all changes in that frame are applied.
- Reconnects create a new source session; each source is intended to run as an independent OS process with its own journal.
- GitHub Actions CI for `go test -count=1 ./...` and Linux live-source compilation.

## Default live endpoints

- M2: `wss://fstream.binance.com/ws/btcusdt@bookTicker`
- Spot: `wss://stream.binance.com:9443/ws/btcusdt@bookTicker`
- PM: `wss://ws-subscriptions-clob.polymarket.com/ws/market`

## Example commands

M2:

```bash
go run ./cmd/live_source --source=m2 --run-id=test-v2 --duration=30s --out=artifacts/test-v2/m2.jsonl
```

Spot:

```bash
go run ./cmd/live_source --source=spot --run-id=test-v2 --duration=30s --out=artifacts/test-v2/spot.jsonl
```

PM Plane A:

```bash
go run ./cmd/live_source --source=pm --plane=A --run-id=test-v2 --duration=30s --out=artifacts/test-v2/pm-a.jsonl --up-token=<UP_TOKEN> --down-token=<DOWN_TOKEN> --tick-size=0.01
```

PM Plane B uses the same command with `--plane=B` and a different output path/process.

## V2 validation boundary

A local `go test` / build PASS proves only parser, type, journal, and compilation integrity. A real GCP no-order run is still required to validate endpoint behavior, reconnect semantics, dual-plane observability, and live timing.

V2 does not authorize strategy logic or trading.
