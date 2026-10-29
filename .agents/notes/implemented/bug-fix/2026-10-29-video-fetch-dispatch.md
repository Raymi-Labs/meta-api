# Agent Note: Preserve resolved video task query modes

Status: implemented

## Problem

Video generation can finish successfully while GET task status returns a nil-function panic. The distributor resolves GET to VideoFetchByID, but base relay info infers VideoSubmit from the same URL prefix. The fetch dispatcher records an unsupported-mode error without returning before invoking the missing builder.

## Existing capabilities and impact

`router/video-router.go` already authenticates both video query URL families. The distributor resolves HTTP methods; the controller builds task relay info; the relay fetches tasks by user ID and returns native/OpenAI-compatible representations. Reuse these paths, stored task results, and existing provider conversion. No new upstream queries or paid generation are needed.

The [video common contract](../../implemented/bug-fix/2026-10-26-video-task-common-contract.md) and [task funding consistency](../../implemented/bug-fix/2026-10-24-task-submission-and-terminal-consistency.md) overlap in lifecycle and billing but do not own request-mode dispatch. Their behavior remains unchanged. The [video pricing proposal](../../proposed/feature/2026-10-21-video-pricing-and-testing.md) remains out of scope.

## Decision

Within task relay info construction, prefer a non-unknown mode already resolved by server middleware over the generic path classifier. Preserve the path fallback when no resolved mode exists. Restrict this override to task format so other formats keep their existing semantics. Return immediately when the task fetch builder is absent.

## Alternatives considered

- Add an HTTP-method-aware global classifier: self-contained but duplicates existing provider-specific middleware classification and expands changes to unrelated relay callers.
- Hardcode VideoFetchByID in the fetch controller: fixes video URLs but the shared controller also serves Suno batch and single-task queries, so a blanket override would select the wrong builder.
- Only return on missing builder: prevents the panic but leaves valid video queries failing instead of returning stored results.

## Testing

The new regression tests failed against the original implementation: both GET URL families resolved to VideoSubmit and the missing builder panicked. After the fix, actual `SetVideoRouter` routes with TokenAuth return queued, in-progress, completed, and failed stored xAI tasks through both representations. Non-owner/missing tasks retain task_not_exist, missing credentials remain unauthorized, private credentials do not appear in results, and upstream call counts, task counts, and user/token quotas remain unchanged.

Table tests cover task creation, legacy creation, remix, Kling fetch, Suno single/batch fetch, missing-mode fallback, and isolation from text relay. Unsupported/unknown modes return a structured 400 without writing a response or panicking.

Full suites passed for `relay/common`, `relay`, all task adaptor packages, `router`, `controller`, `middleware`, and `service`. Focused regressions also passed with the race detector. The complete server compiled with Go 1.26.10. Note validation and whitespace checks passed. No paid upstream calls or production changes are part of this local verification.

## Consequences

Task-format callers must supply a server-resolved mode; HTTP input must not set it directly. Missing modes retain the previous fallback. Deployment and querying the already-generated task are required before production recovery can be claimed.
