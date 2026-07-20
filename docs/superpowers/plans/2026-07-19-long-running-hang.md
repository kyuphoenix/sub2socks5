# Long-running hang hardening plan

**Goal:** Remove the global stalls and stale-state failure modes that can make the management service or proxy appear hung after it has run for a while.

## Root-cause evidence

- Subscription refresh performs remote HTTP requests while holding `App.mu` exclusively.
- Several handlers keep `App.mu`/`RLock` held while encoding and writing HTTP responses; a slow or disconnected client can therefore block a writer and then block all later readers.
- The HTTP server has no read/write/idle timeouts.
- Failed subscription fetches are treated as successful empty subscriptions, so a transient upstream outage can replace healthy cached nodes.
- Kernel downloads have no overall or stalled-read timeout, and response/request bodies are not consistently bounded.

## Tasks

1. Add regression tests proving that slow subscription I/O and slow response writers do not hold the global application lock.
2. Refactor subscription refresh to snapshot configuration under a short lock, perform network I/O without the global lock, serialize refresh jobs separately, and preserve the last good state when every source fails.
3. Snapshot/serialize handler responses before releasing locks so network writes never happen while holding application locks and shared maps are not encoded concurrently with mutation.
4. Configure an explicit `http.Server` with header/read/write/idle timeouts and cap API request bodies.
5. Bound subscription/download response reads and add stalled-download protection.
6. Bound partial child-process log lines and fix restart-attempt accounting so repeated crashes back off correctly.
7. Run formatting, unit tests, race tests, vet, build, and frontend syntax checks.
