# Retired Merlin patches

Everything here is historical. Stock `3006.102-wifi6` builds and flashes
without any of these. `firmware/build.sh` never applies this directory.

Notable:

- `0010-gt-ax6000-httpd-missing-symbol-guards.patch` — removed privacy CGI
  routes; caused HTTP 404 on first-time setup. Never re-apply on wifi6.
- `0011-…privacy-policy-state…` — stubs a symbol wifi6 already ships.
- `0002`–`0009`, `0012`–`0015`, `0017` — 3004-era build fixes / HOSTAPD-off
  workarounds. Not needed once plain `make gt-ax6000` on wifi6 succeeded.
