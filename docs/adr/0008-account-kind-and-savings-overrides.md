# Account kind and counts-as-savings overrides

The account `kind` and `counts-as-savings` overrides ([ADR-0005](0005-spending-tool-three-bucket-account-kind.md)) are surfaced as an inline per-row picker on the accounts overview. Two non-obvious rules fall out:

- **The savings toggle is offered on every kind but `credit`, and overriding an account *to* `credit` force-clears the flag.** The transfer-subtype pairing engine ([ADR-0003](0003-two-layer-transfer-detection.md)) trusts the flag with no kind check, so a credit destination must carry it false — otherwise a card payment would count as saving. This is the one coupling between the two otherwise-orthogonal axes.
- **An effective flag change eagerly re-pairs existing transfers** — through an injected seam, no provider call — so the Tracker reflects it at once rather than at the next sync.

Rejected: a kind-check inside the pure pairing engine instead of clearing the flag (spreads the special case and leaves a contradictory stored flag); go-forward-only re-pairing (the toggle would sit inert for hours after an explicit action).
