---
name: meta-api-video
description: Generate a video through YuanHeng from a natural-language description, or resume an existing video task. Use for explicit video creation requests through meta-api.vip; supports verified Grok and Snow-compatible video profiles. Does not generate images or treat capability questions as paid generation requests.
---

# YuanHeng Video

Use `scripts/meta_api_video.py` with Python 3.9+ (standard library only). Keep the conversation model unchanged. The helper submits JSON once, records the task ID, waits with backoff, and returns the completed video URL. Do not replace it with improvised fetch or multipart code.

## User experience

Accept a description such as “use Grok to generate a video of waves at sunset.” Use the user's specified origin and API key. Supply the key through the child process environment variable `META_API_KEY`; never put it in command arguments, logs, committed files, or the recovery record. Do not search other apps for a key. If the user already provided one for this task, reuse it without asking again. Otherwise ask for normal credential configuration.

Supported models are `grok-imagine-video` (default) and `grok-imagine-video-1.5` (select explicitly with `--model`). The 1.5 model has passed a one-second 480p account-level generation test; this does not verify all accounts or YuanHeng pricing/routing. Additional verified supplier profiles are listed below; other models, editing, and extensions are not implemented by this helper. Preserve explicit user choices; explain an unsupported choice rather than silently substituting. Only the explicitly listed Veo Lite profile accepts a reference image.

If duration/resolution/aspect ratio are unspecified, state the defaults before submission: **1 second, 480p, 16:9**, which have been verified on this route. The CLI accepts 1–15 seconds, 480p/720p (plus 1080p only for the explicitly selected 1.5 model) and 16:9/9:16/1:1 as a limited profile; only the default combination has been live-tested here. Higher settings still depend on the provider and may cost more. Never use paid trials to discover limits.

Actual generation requests authorize a paid submission; questions, installation, dry runs, and resuming a query do not authorize new paid generation. Follow the host's permission mechanism. Do not switch hosts, keys, proxies, or providers to bypass a denial.

## Generate

Resolve the script path relative to this Skill. Use a distinct recovery file for each user-authorized new video; reuse that file for every continuation of the same request. The folder must be private and writable by the current user.

```bash
python3 scripts/meta_api_video.py \
  --base-url https://meta-api.vip \
  --prompt 'A lighthouse at sunrise, soft ocean waves, a slow continuous camera move.' \
  --duration 1 --resolution 480p --aspect-ratio 16:9 \
  --state output/video/lighthouse-task.json
```

Run in a managed tool session. Do not launch a detached background job. The helper emits progress while waiting; poll the managed session only to collect results. It defaults to a bounded 420-second wait and terminates/reaps its worker on timeout or interruption. Increasing `--wait` changes how long to query, not how many tasks to create.

`--dry-run` validates and prints a plan without credentials, network calls, or state-file writes. A dry run is not proof that the key/model works.

## Resume and uncertainty

```bash
python3 scripts/meta_api_video.py --resume --state output/video/lighthouse-task.json
```

- `accepted`, `queued`, `in_progress`, or `pending`: use the same task ID; never submit again to refresh progress. Resume uses the saved origin and original credential. Expired/rotated keys need explicit reconciliation rather than trying alternatives.
- `submission_unknown` or interruption without an ID: the request may have reached the server. Inspect the task/usage logs; do not delete the recovery file or pick a new filename to bypass the guard. Obtain explicit authorization before any replacement paid submission.
- `failed`: report failure without claiming no charge. Do not automatically resubmit.
- Missing output URL, malformed response, or query errors: preserve the ID and report the actual problem. A completed state without a valid result URL is not a deliverable success.
- If the process reports an accepted ID but cannot save it, preserve that ID in the conversation and use read-only task queries; do not create another task.

The recovery file contains origin, a credential fingerprint, parameters, status, task ID and possibly a result URL; it contains no key or prompt. Keep it private. It prevents duplicate starts only when the same file is reused, not across unrelated machines or files.

## Deliver

When `phase=completed`, show the video with the host's supported preview tool. If no preview is available, provide a clickable `video_url`. Do not download remote media to bypass host display restrictions. Do not expose the API key in the preview URL. Describe actual returned information; do not invent resolution, duration, audio, cost, or a successful visual inspection.

Exit 0 means completed (or a successful dry run); 2 means bounded waiting/interruption; 1 means an error. Check the reported phase and recovery file rather than relying only on exit status. Confirm the managed process has exited before finishing. A task may continue on the server after the local wait ends; local interruption is not a server-side cancellation.

## Snow-compatible profiles

Use the same YuanHeng origin and credential. No extra permission or quote request is required by this workflow. Respect actual API errors and never retry a paid POST automatically.

- `flow-video-gemini-omni-flash-1.1`: text-to-video, 4 seconds, 720p.
- `flow-video-gemini-omni-flash-1.1-1080p`: text-to-video, 4 seconds, 1080p (supplier-upscaled output).
- `muse-video`: text-to-video, 4 seconds, 720p.
- `flow-video-veo-3.1-lite`: image-to-video, 8 seconds, 720p; requires `--image` with a user-provided PNG/JPEG. Do not invent a reference or silently use paid image generation. Reference bytes are sent to YuanHeng and its selected upstream; do not include unrelated files.
- `snow-grok-imagine-video`: a distinct Snow upstream alias, 4 seconds, 720p; does not replace the original CPA Grok route.

These profiles default to their verified duration/resolution when omitted and support 16:9 or 9:16. Explicit incompatible settings fail before submission. Other vendor models, editing, and extensions remain unsupported. Gemini/Veo production readiness also requires the channel-scoped video header-wait fix to be deployed; local profile support alone does not prove deployment.

A completed compatible task may provide `content_url` with `requires_authorization=true` instead of a public URL. Fetch that same YuanHeng-origin `/v1/videos/{id}/content` endpoint with the original key using the host's permitted HTTP/file tools, verify HTTP success and video MIME type, save to the current task's output directory, and preview the local file. Never append a key to a URL, send it to a media CDN, or claim a protected link can play without authentication. This fetch retrieves an existing result and does not submit generation. Respect any host restrictions on media display.
