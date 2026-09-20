# Performance & optimisation methodology

Applies to Ethernet, Wi-Fi, VPN and latency work (Milestone 4), and to any manual
tuning before that. The iron rule:

```
OBSERVE → BASELINE TEST → CHANGE ONE THING → TEST → COMPARE → KEEP OR REVERT → CONTINUE
```

No change is kept without a measured, statistically meaningful improvement, and
every change is applied inside a rollback transaction.

## Non-negotiables

- **Never disable Broadcom hardware acceleration accidentally.** Before/after
  every tuning step, record `fc status` (flow cache) and Archer/runner state.
  Many "Linux networking tips" (e.g. some qdisc/QoS/netfilter settings, packet
  capture, some conntrack options) silently punt traffic off the accelerated
  path on HND platforms — that is a 10x routing throughput regression on this
  hardware class. The optimiser must treat "acceleration still enabled and flows
  still being accelerated" as a health check, not just throughput numbers.
- **Loaded latency counts, not just throughput.** Benchmarks record ping/jitter
  under load (bufferbloat), not only iperf3 Gbit/s.
- **Repeatability**: each test runs N times; compare medians; require the delta
  to exceed run-to-run noise before "keeping" a change.

## Ethernet tuning candidates (investigate, measure, never assume)

- hardware NAT / flow cache / Archer state and coverage
- IRQ affinity & interrupt distribution across the 4 cores
- CPU affinity of network-heavy processes
- packet queues, `ethtool` ring/coalescing where the Broadcom driver honours it
- network buffer sysctls, conntrack table sizing (timeouts, max entries)
- firewall chain traversal cost (rule count/order; unnecessary inspection)
- QoS overhead (ASUS QoS engines can disable HW acceleration — measure!)
- PPPoE path (if used on WAN)
- VPN fast paths (wireguard on 4 cores; MTU; UDP offload behaviour)

## Wi-Fi optimisation loop

1. Observe: `wifi.scan` (neighbouring APs, channel utilisation), RSSI/noise per
   client, PHY rates, retry/error counters.
2. Baseline: throughput + latency to a reference wired host from 1–2 reference
   wireless clients.
3. Candidates: channel, width (80 vs 160 on 5 GHz), and supported radio params.
4. Apply one candidate (rollback-armed), re-test, compare, keep or revert.
5. Stability beats peak: prefer the config with the best worst-case, and re-check
   at a later hour before declaring victory (interference is time-varying).

## Benchmark toolkit

- `iperf3` (router as client/server, and through-the-router LAN↔LAN / LAN↔WAN)
- WAN speed test from the router
- `ping` RTT/jitter/loss (idle and under load)
- interface error/drop counters before vs after

## Result recording

Every optimiser run stores to USB: timestamp, config delta, raw measurements,
verdict (kept/reverted), acceleration state before/after. This becomes the input
for "AXOS Performance Mode" and lets the AI answer "why is this setting what it
is" with data.
