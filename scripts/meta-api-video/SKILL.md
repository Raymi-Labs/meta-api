---
name: meta-api-video
description: Generate YuanHeng videos from text or reference images, resume existing tasks and download completed MP4s without resubmitting. Supports Muse, Grok and verified Omni profiles; questions or installation do not authorize paid generation.
---

# YuanHeng video

Use `scripts/meta_api_video.py` with Python 3.9+ and standard-library dependencies. Keep the conversation model unchanged. Use the user's YuanHeng origin (default https://meta-api.vip) and `META_API_KEY` from the child process environment. Never put credentials in command arguments, saved state, logs or CDN URLs. No extra permission/quote API lookup is required by this workflow.

## Supported input and acceptance boundaries

- Muse `muse-video`: integer requests 4–180 seconds, default 4, 720p, 16:9 or 9:16. One first frame via `--image`, or 1–4 ordered files via `--images`, never both. PNG/JPEG/WebP, at most 10,000,000 bytes each and 24,000,000 total; prompt at most 4000 characters.
- The first Muse image is the first frame; later images guide subjects, scenery or style, not the final frame. Do not silently resize, omit, combine or replace references.
- Muse 60-second production acceptance through this YuanHeng workflow completed in 526 seconds, yielding an actual 60-second 1280x720/24fps MP4 with one audio track. Four inputs including WebP were accepted, but later reference elements did not visibly appear in sampled frames. Do not claim reliable four-image adherence. 180 seconds remains supplier-declared, not live-verified here.
- Muse above 30 seconds is supplier-stitched from shorter segments, not a guaranteed continuous shot. Describe scenes in roughly 30-second sections. Actual durations can round upward; inspect the file rather than treating task seconds/size as output proof.
- `grok-imagine-video`: 1–15 seconds, 480p/720p, default 1 second/480p/16:9 in this standalone package. The user's installed combined Skill retains its 5-second default. Optional PNG/JPEG `--image` is a first frame; a 1-second production test succeeded, actual 1.042 seconds, 736x400, 24fps.
- `grok-imagine-video-1.5`: explicit selection, allows 1080p; recent image-to-video attempt returned upstream saturation (429), so do not claim live acceptance for that mode. Grok uses the input image aspect ratio.
- `flow-video-gemini-omni-flash-1.1`: 4 seconds/720p, text only; full YuanHeng chain verified, actual 4-second 1280x720/24fps.
- `flow-video-gemini-omni-flash-1.1-1080p`: 4 seconds/1080p, text only; full chain verified, actual about 3.92 seconds/1920x1080/24fps, supplier-upscaled.
- `snow-grok-imagine-video`: separate supplier alias, text only, 4 seconds/720p.
- Veo Lite is retired: reject new submissions, retain GET-only recovery of existing tasks. Do not substitute it or reopen unrelated blocked providers.

When a user asks to animate an image without naming a provider, prefer the verified Muse single-image path and state the choice. Preserve explicit model choices. Do not infer broad stability from one successful sample or from an unspecified user report.

## Generate and deliver

Use a distinct private recovery file and a new output filename for each explicitly authorized video. Include `--out` for normal delivery so a completed status alone is not the final result.

```bash
python3 scripts/meta_api_video.py --model muse-video --duration 60 \
  --images first.png character.jpg --resolution 720p --aspect-ratio 16:9 \
  --prompt 'Two scenes of about 30 seconds each: ...' \
  --state output/video/task.json --out output/video/result.mp4
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
