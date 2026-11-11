---
name: meta-api-video
description: Generate YuanHeng videos from text or reference images, resume existing tasks and download completed MP4s without resubmitting. Maps Muse, Grok, Seedance 2.5, Omni and explicit compatible model IDs; questions or installation do not authorize paid generation.
---

# YuanHeng video

Use `scripts/meta_api_video.py` with Python 3.9+ and standard-library dependencies. Keep the conversation model unchanged. Use the user's YuanHeng origin (default https://meta-api.com) and `META_API_KEY` from the child process environment. Never put credentials in command arguments, saved state, logs or CDN URLs. No extra permission/quote API lookup is required by this workflow.

## Parameters and request mapping

Preserve the user's model ID, duration, resolution, aspect ratio and references. A prior successful sample is evidence, never a capability gate. Defaults are preferences, not upper limits. Let the endpoint report unsupported settings; never downgrade, switch protocols or submit again automatically.

- Known Grok IDs default to xAI-style `duration/resolution/aspect_ratio/image`; standalone Grok defaults to 1 second/480p/16:9 (the installed combined Skill retains 5 seconds).
- Seedance `doubao-seedance-2-5-260628` defaults to 4 seconds/720p/16:9 and uses `seconds/resolution/metadata` plus `images` references.
- Muse and compatible Omni/Snow models use `seconds/resolution/aspect_ratio` and `input_reference` or `images`. Defaults remain 4 seconds/720p, except the named Omni1080p alias defaults to1080p. Standard size pairs are derived; other resolution/ratio tokens pass through without guessing pixel dimensions. `--size WIDTHxHEIGHT` is available when the endpoint needs explicit pixels.
- Other full model IDs use the unified OpenAI video request format. Set `--protocol openai|xai|seedance` when the required format is known. `--protocol auto` selects only from existing known mappings; one POST is made to `/v1/videos` with no protocol fallback.
- Positive integer duration and non-empty resolution/ratio format tokens are accepted. `--image` and `--images` encode local PNG/JPEG/WebP files; reference order is preserved. Keep local file/memory limits, not unverified provider limits.
- Veo Lite remains retired by the user's explicit decision: existing tasks can be recovered, new submissions are rejected.
- New tasks default to `https://meta-api.com`; explicit legacy YuanHeng origins remain valid. Resume/download-only retain the original origin and credential. Completed output shows only the original gateway content URL and `requires_authorization:true`, never provider URLs.

Previous samples established selected Muse/Grok/Omni/Seedance generation and download paths, not every parameter combination. State an actual failure if the endpoint rejects a request; do not charge for extra exploratory tests unless asked. The user's key must have access to the selected model's group (Seedance currently enterprise).

## Generate and deliver

Use a distinct private recovery file and a new output filename for each explicitly authorized video. Include `--out` for normal delivery so a completed status alone is not the final result.

```bash
python3 scripts/meta_api_video.py --model muse-video --duration 60 \
  --images first.png character.jpg --resolution 720p --aspect-ratio 16:9 \
  --prompt 'Two scenes of about 30 seconds each: ...' \
  --state output/video/task.json --out output/video/result.mp4
```

Seedance example (same automatic polling and MP4 delivery):

```bash
python3 scripts/meta_api_video.py --base-url https://meta-api.com \
  --model doubao-seedance-2-5-260628 --duration 4 --resolution 720p \
  --prompt 'A white paper boat floating on a quiet pond.' \
  --state output/video/seedance-task.json --out output/video/seedance.mp4
```

Only one POST is issued. The task ID is saved before polling. Default wait is 1800 seconds for Muse over 30 seconds, otherwise 420; explicit --wait wins. Run in a managed tool session, never detached. Polling uses bounded backoff; up to three consecutive explicit unknown statuses with a matching ID are tolerated without another POST.

`--out` streams the authenticated original-origin task content endpoint to a private temporary file, refuses redirects/non-MP4 responses and overwrites, and inspects actual MP4 duration, dimensions and audio-track count before publication. Default download budget is 900 seconds, independent from generation wait; maximum file size is 512 MiB. Metadata does not prove visual quality or audible sound: preview the downloaded media and report material differences. Never regenerate merely because the output differs.

## Recover without duplicate charges

```bash
python3 scripts/meta_api_video.py --resume --state output/video/task.json \
  --out output/video/result.mp4

python3 scripts/meta_api_video.py --download-only --state output/video/task.json \
  --out output/video/recovered.mp4 --download-timeout 900
```

Resume queries the original task using the original origin/key fingerprint; source images are no longer needed. Download-only requires completed state and performs only a content GET, never a POST or generation poll. A failed download leaves completed state intact. Use a new output path if one already exists.

A submission_unknown state without an ID requires task/log reconciliation before another paid request. Failed/unknown tasks do not prove zero charges. Do not change keys, providers, proxies or origins to bypass errors. Download retries retrieve the original result only; do not use HEAD or rely on Range support. Clean only this invocation's temporary files after verifying its workers exited. User content, credentials and image data are not persisted in recovery state or dry-run output.

`--dry-run` validates without credentials, network, state writes or generation. It is not upstream availability proof. The supervisor terminates and reaps its worker on interruption/deadline; confirm the command itself exited before reporting zero remaining workers.

## Pricing evidence (2026-10-31)

Prices remain server-side. With media group 0.25 and normal recharge RMB0.20/credit, current Muse retail is 3 credits/requested second (RMB0.60): 60 seconds costs 180 credits, 180 seconds 540. Supplier cost is $0.20 balance/requested second and the user reports RMB1 per $1 supplier balance. The prior 60-second acceptance cost 120 credits at the old rate and $12 supplier balance; it is not retrospectively repriced. No per-image supplement was observed.

Omni720p and the separate Snow Grok alias currently cost 0.75 credits/second, Omni1080p 1.5. Do not apply the Snow cash conversion to CPA Grok or other providers. Cash margins also depend on actual recharge discounts, referral payouts and fees. Supplier-rounded durations and every long-video tier are not exhaustively live-verified.


Grok 1.5 single-image recheck on 2026-10-31 succeeded through YuanHeng: one 1-second/480p request, actual 1.042 seconds, 736x400, 24fps and one audio track. Reference shapes, colors and marks persisted; channel 1577 charged 1.5 credits. Other specifications are not implied. Muse 180 seconds is supplier-guaranteed per the user, not independently verified here; multi-image quality remains unresolved.
