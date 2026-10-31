# Agent Note: Provider-scoped video reference inputs

Status: implemented

## Problem

Production Muse accepts input_reference and produces a visually verified video, but clients reject optional references. Grok fails before submission because the shared task DTO expects a string while xAI requires an image object with url.

## Decision

Normalize xAI references only in its adaptor, accepting native image.url and string/input_reference aliases. Reject malformed or conflicting inputs before submitting. Reuse shared validation, context storage, billing, authentication and polling without weakening other providers' DTOs. Extend both video clients with optional Muse/Grok PNG/JPEG references and rebuild archives. Keep Grok production support explicitly unverified until deployment and live acceptance.

## Alternatives considered

- Make shared Image an arbitrary object: rejected because unrelated providers would inherit a weaker contract.
- Silently forward competing reference fields: rejected because images might be ignored in paid generation.
- Add a separate endpoint: rejected because the existing task lifecycle already supports Muse references.

## Testing

Adaptor and shared relay tests pass for native image objects, aliases, invalid shapes, conflicts, text-only requests and unchanged non-xAI parsing. The first-frame integration test checks validation, outgoing body and duration-based billing together. Standalone client tests (31) and installed client tests (35) pass, including reference encoding, dry-run/state privacy, single submission, resume without the source file and worker cleanup. Go compilation succeeds. Both allowlisted archives match their source files.

## Consequences

Upstream capability and output remain provider-dependent. The Muse four-second request produced five seconds at 720p/24fps. Local tests do not prove deployment or Grok production availability.

This extends the [existing video client lifecycle](../feature/2026-10-29-video-generation-skill.md) without replacing its submission/recovery decisions. The shared string DTO stays unchanged; xAI native file_id objects and arbitrary image-object properties remain unsupported rather than being silently dropped.
