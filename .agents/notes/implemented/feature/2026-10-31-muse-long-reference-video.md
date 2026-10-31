# Agent Note: Documented Muse long and multi-reference video requests

Status: implemented

## Problem

The supplier document supplied by the user declares Muse requests of 4–180 seconds and up to four PNG/JPEG/WebP references. The installed and distributed clients still reject requests above 30 seconds and multiple references. Earlier live tests rejected 32 seconds, so the new document is a changed contract, not proof of current production capability.

## Decision

Extend the existing JSON task lifecycle instead of adding a new endpoint. Muse clients accept one first frame or up to four ordered references, enforce 10 MB per image and 24 MB total, validate 4000-character prompts, and default to a 30-minute wait for requests above 30 seconds. Explicit waits still win. The Muse-specific server JSON path normalizes string/numeric seconds and validates the same structural limits before forwarding without changing unrelated Sora providers. Retain streamed authenticated content download. Extend only the production Muse duration guard, retaining its unit price and group multiplier, and label long/multi-image modes supplier-declared until live acceptance.

## Alternatives considered

- Treat 30 seconds as a permanent provider limit: rejected because new supplier evidence explicitly changes the supported contract.
- Automatically split and submit multiple paid tasks: rejected because the supplier owns stitching and bills a single request; retries would duplicate charges.
- Globally loosen all video parameter validation: rejected because Grok and other providers use different image and duration contracts.

## Testing

Tests pass for 31/60/180-second payloads, 181/fractional/conflicting duration rejection, ordered four-image encoding, five-image rejection, single/total image size caps, WebP and conflicting reference fields. Existing GET-only recovery and private diagnostics tests remain passing. Both video client suites pass (36 standalone and 40 installed). Sora adaptor, common relay, relay and router suites pass; the integration fixture verifies 180-second billing with four references. Go compilation succeeds. The production pricing guard was changed only from 30 to 180 and confirmed after page reload; unit rate stays unchanged. No paid generation was submitted.

## Consequences

Supplier segmentation above 30 seconds is not continuous-shot generation. Live 180-second availability, multi-reference adherence and actual video durations remain unverified. Price units are upstream balance, not necessarily RMB. An increased client wait does not override deployment-specific server task expiration or CDN limits.

JSON is the tested integration path; this change does not expand multipart validation or fetch remote references to measure size. The server default task timeout is 1440 minutes in code; deployment environment overrides were not inspected. Streamed downloads already use io.Copy. Local code and share archives are updated, but backend changes are not pushed or deployed in this task.


## Observed acceptance and current retail

A single 60-second, four-reference request through the YuanHeng Skill completed in 526 seconds and downloaded successfully: actual 60 seconds, 1280x720, 24fps, one audio track. The supplier charged $12 balance; the site charged 120 credits under the then-current retail rate. The subsequent retail adjustment is 3 credits/requested second (RMB0.60 at normal recharge), so a new 60-second request costs 180 credits. Old requests are not repriced. Four references including WebP were accepted but sampled frames did not visibly follow references 2–4; acceptance is not a quality guarantee. 180 seconds remains untested because supplier balance before the test was $30.34, below the $36 precharge.
