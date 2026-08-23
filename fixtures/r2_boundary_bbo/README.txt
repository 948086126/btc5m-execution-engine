V0 regression target:
- embedded best_bid == 0 => natural empty bid (null)
- embedded best_ask == 1 => natural empty ask (null)
- embedded best_bid == 1 remains invalid
- embedded best_ask == 0 remains invalid
- non-empty executable prices must be strictly inside (0,1) and tick-aligned
