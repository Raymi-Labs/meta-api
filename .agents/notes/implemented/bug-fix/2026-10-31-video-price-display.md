# Agent Note: Video prices and task billing evidence

Status: implemented

## Problem

Video catalog entries hide per-second prices behind an expression label. Task logs write model_price=0 even when expression billing charged a nonzero amount, producing misleading zero-price formulas and upstream attribution.

## Decision

Recognize only explicit linear duration tiers for catalog summaries with existing group/currency conversion; retain fallback for unknown expressions. Capture a separate task-log snapshot at original price evaluation without changing settlement. Old video logs show their recorded debit only, while new snapshots also show requested duration and effective per-second debit. Never persist prompts, images or headers.

## Alternatives considered

- Evaluate arbitrary expressions in the browser: rejected for safety and semantic mismatch.
- Recalculate historical charges at current prices: rejected because rates change.
- Reuse the text settlement snapshot: rejected because it affects settlement behavior.

## Testing

Targeted frontend tests cover linear rates, ranges, expression-wide arithmetic fallback, group/currency conversion, old video debit rendering and new snapshot details (13 tests plus existing pricing assertion suites). Go tests cover task snapshot values, no prompt/image leakage, unchanged settlement snapshot and stable historical values after pricing edits. Related task/tiered tests and Go compilation pass. The full frontend build includes language completeness checks; prerender skips under existing behavior when local Chromium is absent. No billing rate or historical ledger is changed by this patch.

## Consequences

Rate ranges describe priced branches, not availability. Historical missing parameters cannot be reconstructed. New snapshots require deployment.
