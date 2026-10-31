# Agent Note: Preserve omitted settings during channel updates

Status: implemented

## Problem

Production channel 1580 lost its 600-second header wait when enabled. List actions send only ID and status/priority/weight. UpdateChannel nevertheless normalizes the empty setting through applyChannelQuotaPolicyAnchor, serializing defaults and overwriting the stored setting. Proxy, identity, concurrency and quota policy can also be erased.

## Decision

On existing-channel updates, leave a nil Setting untouched and skip quota-anchor normalization. GORM already omits nil pointers in struct updates, so avoid copying a stale snapshot into the patch. Explicit setting strings continue through existing validation and anchor rules; creation retains its defaults. JSON null has the existing pointer semantics of omission; an explicit empty string or object resets settings.

## Alternatives considered

- Send full settings from each frontend action: protects one client but leaves other API callers vulnerable and introduces stale writes.
- Copy origin settings into every patch: retains known values but rewrites a field the caller did not touch and can lose concurrent changes.
- Skip normalization whenever quota is disabled: would fail explicit resets and unrelated settings changes; presence, not quota state, is the boundary.

## Testing

Handler-level SQLite database tests cover enable, disable, priority and weight mutations while preserving raw setting bytes and active quota anchors. Explicit settings can still change timeout/proxy, preserve unchanged quota windows, reset changed windows and disable quotas. Existing manual recovery and tag operations continue working. Full controller tests, targeted race checks, server compile and note verification pass.

## Consequences

This does not repair settings already overwritten or provide new partial merging inside a submitted setting JSON string. The separate video wait behavior remains in its existing owner note.
