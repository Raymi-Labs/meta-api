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


## Production acceptance, 2026-10-31

After deployment the first Omni720p request still timed out at 60 seconds. Readback showed the configured header wait had returned to zero after status toggling. The status-only patch invokes quota-anchor normalization on an empty setting, which can serialize defaults; this separate persistence defect remains unresolved. Enabling first and then saving the 600-second setting preserves the override. Omni1080p subsequently passed submit, query and content retrieval (1920x1080, 3.92 seconds, 24 FPS). Channel 1580 now exposes only that model with the wait read back as 600. Omni720p's ambiguous no-ID submission was not replayed. Veo Lite completed but authenticated content retrieval receives upstream 403, surfaced as gateway 502; it remains withheld. Do not claim deployment alone establishes every supplier flow.


## Reconciled acceptance, 2026-10-31 evening

The current deployed version includes the partial-setting preservation fix; channel 1580 retains 600 seconds. The upstream daily task and consumption listings contain no record matching the earlier ambiguous Omni720 request. A new explicitly authorized acceptance request succeeds through YuanHeng and yields a verified 4-second 1280x720/24fps video, costing 0.20 upstream balance units and 2 site credits. Both Omni resolutions remain enabled. Veo Lite is still withheld: querying the original upstream task succeeds, but its authenticated content endpoint returns 403 with "request blocked: port 8810 is not allowed". No replacement Veo generation is submitted.
