# Agent Note: Channel-scoped waits for synchronous video submissions

Status: implemented

## Problem

Snow Gemini video submissions can wait more than 60 seconds before returning an ID. A direct 1080p request took 224 seconds while the YuanHeng relay failed at 60 seconds; resubmission would risk duplicate charges. Existing channel timeout overrides exclude video submissions.

## Existing capabilities and impact

Reuse the [channel header override](../../implemented/feature/2026-10-05-channel-nonstream-header-timeout.md), its bounded client cache, and the existing no-replay rule for started video requests. Scope changes to VideoSubmit through the common executor; background polls and other formats retain their existing clients. The new channels use the Sora-compatible adapter for `/v1/videos`.

## Decision

Allow the existing explicit channel nonstream header setting to apply to video submission. Keep zero as inheritance, global settings unchanged, and preserve total timeout, proxy, TLS, cancellation and paid-submit retry rules. Update the UI explanation and supporting documentation without changing other agents' pending work.

## Alternatives considered

- Raise the global 60-second timeout: immediately available but affects every unrelated channel, so reject.
- Resubmit after the timeout: risks duplicate paid tasks, so reject.
- Invent an async flag on the supplier endpoint: no verified contract supports it; avoid speculative forwarding.

## Testing

A real local slow HTTP fixture succeeds with a VideoSubmit channel override and fails without it; shared global client and query modes remain unchanged. Relay/channel, service and controller suites pass; targeted timeout regressions pass with the race detector. Server compile, UI checks and note gates are verified locally. Production remains pending deployment; configuration alone does not establish recovery.

## Consequences

Longer waits occupy connections; CDN and total timeouts can still be shorter. The upstream may accept a request before the client loses its ID. No automatic replay is introduced.
