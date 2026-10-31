# Agent Note: Initial video task status and bounded client recovery

Status: implemented

## Problem

A production Grok task is accepted, but its first GET can report unknown before polling updates the persisted task. The video client stops although a later GET completes successfully. Task creation initializes NOT_START, while ToVideoStatus omits this known state. Existing Google acceptance records also show an ambiguous Omni720 submission and a completed Veo task with blocked content; neither warrants another paid POST.

## Decision

Map NOT_START to queued in the shared video response, preserving genuinely unknown states. In both video clients, tolerate at most three consecutive explicit unknown statuses with the same task ID through existing bounded GET backoff, then stop with the original state intact. Do not infer success, retry submission, or accept arbitrary status strings. Update both archives and preserve current provider verification caveats.

## Alternatives considered

- Treat every unknown status as pending indefinitely: rejected because broken protocols would look healthy.
- Retry POST after an early query error: rejected because the accepted task already exists and can incur duplicate charges.
- Fix only the client: rejected because the server has an omitted, well-defined initial state.

## Testing

All task-to-video mappings are covered. Model, xAI adaptor and router suites pass. Client suites verify successful recovery after explicit unknown, rejection after the bounded allowance, unchanged mismatched-ID checks and exactly one POST (33 standalone and 37 installed tests). Archive parity and Go build are checked. No new paid task is submitted during this follow-up.

## Consequences

The short compatibility allowance consumes GET time but remains within the original deadline. It does not repair upstream quota, download authorization, missing IDs or Google provider availability.
