# HowlFrame Dogfooding Findings

This document records issues discovered while building HowlBot, an event-driven Discord integration.

### E - application bug (HowlBot)
- The initial Discord role to HowlFrame payload mapping required a translation layer since HowlFrame lacks a native set/array intersection operator for role mapping in our current basic setup. Handled by passing a comma-separated string for simplicity in the adapter.

### B - ergonomics issue (HowlFrame)
- **JSON array parsing**: Working with JSON arrays as `(map_get)` results in strings rather than iterable lists in the simplistic `(parse_json)` output (depending on HowlFrame runtime version). A native JSON destructuring syntax would be ergonomically superior.
