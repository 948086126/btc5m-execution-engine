# V1 Local Mock-Live Release Notes

V1 is still **no-order and no-Internet**. Its purpose is to exercise the same normalized event, timing, journal, and replay core under local transport pressure before any real Binance or Polymarket connector is added.

## Added

- Loopback-only RFC6455 mock WebSocket transport implemented with the Go standard library only.
- Mock Binance Perp (M2), Binance Spot, PM Plane A, and PM Plane B feeds.
- OS monotonic clock implementation:
  - Linux: `CLOCK_MONOTONIC`.
  - Windows: `QueryPerformanceCounter`.
- Per-source asynchronous bounded writer with explicit backpressure fail-closed behavior.
- PM_A abrupt disconnect/reconnect fault injection.
- PM_B configurable delivery delay.
- High-rate M2 flow.
- Repeated legal PM embedded-BBO empty-side sentinels (`best_bid=0`, `best_ask=1`).
- Deterministic replay validation across all four journals.
- Corrupted journal detection test.
- Backpressure detection test.

## Still forbidden / absent

- Internet-facing live connectors.
- Wallets/private keys.
- Order placement, cancel, merge, or funds.
- M2/T1 strategy logic.
- PnL or Alpha claims.

## V1 PASS means only

The local transport/journal/replay machinery survives the frozen mock faults without silent drops or evidence corruption. It does **not** mean the GCP live collector or any strategy is validated.
