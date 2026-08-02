# Record architecture decisions in ADRs

Architectural decisions are recorded as short files in `docs/adr/`, one file per decision, using the format described in the [directory README](README.md).

Without a durable log, the rationale for a decision disappears once the old approach is gone: a future reader has no way to know whether a choice was deliberate, a leftover, or the result of a constraint that no longer applies. A short record written at decision time answers *"why is it like this?"* without archaeology.

ADRs are intentionally minimal — only the decision and the context needed to understand it. Implementation detail that a routine refactor would invalidate belongs in the code or its comments, not here. This file is itself the worked example of the format.
