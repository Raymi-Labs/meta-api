# Agent Note: One-command video generation with resumable task state

Status: implemented

## Problem

Users can ask an agent to generate an image, but video clients must invent submission and polling code. A real agent used multipart for the JSON-only xAI adapter and lost useful diagnostics on a connection failure. The public video guide currently presents multipart as a generic request for every model. Retrying an uncertain submission can charge twice.

## Existing capabilities and impact

The authenticated `/v1/videos` POST/GET routes, background task polling, quota accounting, and result URLs already exist. The [query dispatch fix](../../implemented/bug-fix/2026-10-29-video-fetch-dispatch.md) preserves task status retrieval. The [common video contract](../../implemented/bug-fix/2026-10-26-video-task-common-contract.md) owns provider conversion and paid-submit retry protection. Reuse both. The [video pricing proposal](../../proposed/feature/2026-10-21-video-pricing-and-testing.md) remains separate from this client workflow.

The image workbench is a separate app with current unrelated output changes. This scope targets the user's demonstrated agent + API-key workflow, not a new graphical video editor or synchronous server endpoint. Public handbook Markdown feeds both docs and main web builds; existing static downloads provide distribution.

## Decision

The project ships a standalone Python-standard-library video Skill with a deterministic ZIP. Default to the verified Grok JSON text-to-video request, with explicit duration, resolution, and ratio. The agent handles the user's natural-language prompt while the script performs one POST, persists a private recovery record, polls the same task with bounded backoff, and returns a safe HTTPS video URL. Keep credentials in META_API_KEY, never in arguments or recovery records.

The client uses an exclusive recovery-file reservation before submission to reject repeated/concurrent starts. Mark unknown submission before sending; without an ID, stop and require log reconciliation rather than retry. Persist the ID immediately and bind resume to the original origin and credential fingerprint. A bounded worker process lets the parent terminate and reap stalled work. Polling errors never resubmit or switch providers. Dry runs validate without credentials or network.

## Alternatives considered

- A synchronous server wrapper hides task mechanics but adds long connections and ambiguous client retries; retain the durable asynchronous API and hide polling in the client.
- Only document curl commands fixes the format example but every agent still recreates retry/state handling; provide a runnable helper plus accurate docs.
- Extend the image workbench now would require video storage, previews, routing, and UI work unrelated to the demonstrated conversation failure; keep that future work separate.

## Testing

Twenty-two standard-library tests pass, including real local HTTP JSON submission and status retrieval, resume without POST, uncertain/missing submission ID, concurrent recovery-file reservation, credential/origin binding, wrong response ID, terminal failure, transient queries, malformed completion, secret redaction, private record permissions, and dry runs. A subprocess fixture verifies that the actual parent timeout terminates and reaps a hanging worker. A separate test extracts and executes the public archive without the source tree or credentials. The explicit 1.5 model selection and 1080p profile pass dry-run regression; the original model rejects 1080p before submission; the installed combined image/video Skill passes its own 26-test suite while preserving its user-selected five-second default.

The Skill structure validator and deterministic archive check pass. Public documentation tests pass (14), all content-language tests pass (13), and the documentation site production build succeeds with only the existing large-chunk advisory. Five new public text segments have translations for all 16 non-default site languages. Project note validation and whitespace checks pass.

Tests use localhost fixtures only; no live generation, production configuration change, or paid request is performed. The existing image workbench and other agents' invitation UI changes are preserved. The downloadable file ships through the main site's static assets; deployment of the main site is needed before the new public download URL is available.

## Consequences

Text generation supports grok-imagine-video and explicit grok-imagine-video-1.5; the default model remains unchanged. Both use the same JSON task contract, with one-second 480p account-level evidence for 1.5. The explicit 1.5 profile accepts 1080p per the official text-to-video specification; the original model rejects it before submission. Defaults are preserved. 1080p has offline validation only, not live account acceptance. Adding a channel model does not establish selling prices or prove every pooled credential works; pricing remains an operator setting. The operator requested 1.5 output prices at 10 times official rates: USD 0.80/1.40/2.50 per second at 480p/720p/1080p, with group multipliers applied separately and no inherited minimum charge. This is text-to-video output pricing, not a claim of reference-input cost coverage. Other protocols need explicit profiles. A recovery file prevents repeat submission only when reused, not globally across independent machines. Ambiguous submissions without IDs still require server log reconciliation. Availability and charges remain provider-dependent; never label a pending or failed task successful.

## Verified supplier profiles (2026-10-30)

The helper also supports Snow-compatible Omni 720p/1080p, Muse, Veo Lite with a PNG/JPEG reference, and a separate Snow Grok alias. These profiles use seconds/size and fixed verified settings, leaving Grok defaults unchanged. Veo Lite uses inline image data because the supplier disables remote media fetching. An exhausted test key prevented Muse GET despite successful generation; the permanent model-restricted unlimited key fixes that query boundary without repeated generation.

Compatible completion can expose a top-level URL or only a same-origin authenticated content endpoint. Never send an API key to a returned CDN. The Skill explains authenticated retrieval and preview using host-approved tools. Invalid profile parameters stop before POST. Submission can wait up to the bounded worker deadline instead of the previous 30-second socket limit, while queries retain bounded backoff. The [video wait fix](../bug-fix/2026-10-30-video-channel-wait.md) is necessary for slow Google-compatible production submissions; no global timeout was changed.

The repository helper passes 25 offline tests and the installed combined Skill passes 29, including compatible payloads, required reference image handling, and protected-content completion. Partial supplier failures (429 saturation, missing media mode, unavailable accounts, authentication errors) do not promote a model to the supported list. New video-only channels use Sora type so automatic tests remain free preflight.
