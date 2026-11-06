# GPT Image 2.6 GIF API

## Direct use

Use your existing Meta API key and URL. No Skill or extra authorization/estimate endpoint is required. Your key must have model/group access and sufficient quota, just as for static images.

```bash
export META_API_KEY='your-meta-api-key'
curl --fail-with-body --max-time 240 https://meta-api.vip/v1/images/generations \
  -H "Authorization: Bearer $META_API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{"model":"gpt-image-2.6-sunburst","prompt":"A cute cartoon cat waving its paw, then returning to its starting pose. Fixed camera, white background, seamless looping animation.","output_format":"gif","response_format":"b64_json","n":1}' > result.json
python3 - <<'PY'
import base64, json
from pathlib import Path
result = json.loads(Path('result.json').read_text())
raw = base64.b64decode(result['data'][0]['b64_json'], validate=True)
if raw[:6] not in (b'GIF87a', b'GIF89a'):
    raise SystemExit('Response is not GIF; do not rename static images or resubmit automatically.')
Path('result.gif').write_bytes(raw)
PY
```

`gpt-image-2.6-flare` uses the same API. Save original bytes, not a PNG conversion or the first frame. A GIF signature alone does not prove animation: validate multiple frames.

## Recorded rollout scope (2026-11-05)

Channel 1652 uses a fixed animation preset when output_format is exactly gif: 512x512 requested, medium quality, 9 frames, 200 ms per frame, one output. Other requested frame counts, sizes and qualities are overridden. Actual dimensions can differ; report the file's dimensions. Non-GIF requests retain the previous 1024x1024/high/single PNG policy.

The GIF is one billable output, not nine images. 生图和视频 currently costs 0.5 lightning per request (CNY 0.10 at standard recharge); other groups use their own multiplier. Supplier costs and future prices are not fixed by this API document.

Only text-to-GIF via /v1/images/generations is verified. Editing, SVG, arbitrary frames, long animations, streaming GIF events and alternate upstream failover are not covered. Do not blindly retry a timed-out paid POST.

From v1.23.0, the Images JSON DTO preserves integer `frame_count` and `frame_duration_ms` fields. Channel overrides still take precedence: this does not automatically enable arbitrary frames or change production configuration. API clients can use this document without installing a Skill.
