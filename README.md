# BTC5M Execution Engine — V1 Local Mock-Live

V1 is deliberately **not a trading system**. It validates the transport/journal/replay core under local fault injection before any Internet-facing connector is added.

## What V1 contains

- Typed normalized events.
- Polymarket embedded-BBO boundary semantics regression guard.
- Append-only hash-chained NDJSON journals.
- Nested payload structure that cannot overwrite writer-owned sequence/hash fields.
- Fail-closed journal verifier.
- Deterministic multi-source replay ordered by local monotonic receive time.
- Windows/Linux monotonic clock implementations.
- Loopback-only mock WebSocket feeds for:
  - Binance Perp / M2
  - Binance Spot
  - PM Plane A
  - PM Plane B
- Fault injection for:
  - abrupt PM_A disconnect/reconnect
  - PM_B delivery delay
  - legal PM `best_bid=0` / `best_ask=1` empty-side states
  - high-frequency M2 flow
  - bounded writer backpressure
  - journal corruption

No third-party Go packages are required.

## What V1 explicitly does NOT contain

- Wallets/private keys.
- Order placement/cancel/merge.
- Real Binance or Polymarket Internet connectors.
- M2 alpha logic.
- T1/TWAP logic.
- Strategy state machine.
- PnL claims.

## Windows local verification

From PowerShell in the project directory:

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\verify.ps1
```

This performs three layers:

1. `go test -count=1 ./...`
2. Core fail-closed regression verification.
3. A six-second loopback mock-live run with four concurrent sources and injected faults.

Key expected lines:

```text
R2_BOUNDARY_CASE_PASS
R3_COLLISION_CASE_PASS
JOURNAL_INTEGRITY_PASS
CORRUPTION_DETECTED_PASS
MONOTONIC_CLOCK_PASS
BACKPRESSURE_FAIL_CLOSED_PASS
ALL_TESTS_PASS
MOCK_M2_HIGH_RATE_PASS
MOCK_SPOT_STREAM_PASS
MOCK_PM_BOUNDARY_0_1_PASS
MOCK_RECONNECT_PASS
MOCK_PM_DUAL_PLANE_PASS
MOCK_DETERMINISTIC_REPLAY_PASS
MOCK_LIVE_ALL_PASS
```

The mock run evidence is written to:

```text
artifacts/local-mock-live-verify/
```

including four independent source journals and `MOCK_LIVE_RECEIPT.json`.

## Build Windows and Linux binaries

```powershell
powershell.exe -NoProfile -ExecutionPolicy Bypass -File .\scripts\build.ps1
```

or on Linux:

```bash
./scripts/build.sh
```

## Direct local mock-live run

```powershell
go run ./cmd/mock_live --duration=10s --out .\artifacts\my-mock-run
```

The V1 mock WebSocket client refuses non-loopback hosts by design. Real network connectivity belongs to the next separately authorized increment.

## Next increment after local V1 PASS

V2 should add real **no-order** Binance/PM connectors behind the same interfaces, first tested locally against fixtures and then deployed to GCP. Strategy logic remains a later increment.
"# btc5m-execution-engine"  
