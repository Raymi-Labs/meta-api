# Agent Note: Resumable video artifact delivery

Status: implemented

## Problem

The client preserves task IDs but delegates content download to ad hoc tools. Long tasks need streamed delivery, an independent download budget, actual media inspection, and a download-only retry that cannot create another paid task. Evidence and pricing guidance also lag the verified 60-second Muse result and subsequent price adjustment.

## Decision

Add --out and --download-only to both clients. Download only the authenticated original-origin task content endpoint, never a returned CDN with credentials; refuse redirects and existing output paths. Stream to a private temporary file, check MP4 metadata and publish without overwriting. Keep completed task state even on download failure. Inspect duration, video dimensions and audio-track presence using bounded MP4 box traversal; visual quality remains a separate check. Retire Veo from new submissions but retain GET-only recovery. Rebuild allowlisted archives, synchronize factual acceptance notes and verify the deployed download bytes separately.

## Alternatives considered

- Treat a completed task URL as delivery: rejected because protected URLs and download failures leave users without media.
- Fetch arbitrary returned URLs with the API key: rejected because it leaks credentials across origins.
- Generate again after download failure: rejected because the original paid task exists.
- Add a mandatory external ffmpeg dependency: rejected for portable installation; bounded MP4 metadata inspection covers the required basic checks.

## Testing

Both client suites cover failed download followed by successful GET-only recovery, same-origin credential routing, refusal to overwrite, bounded MP4 parsing and incorrect MIME/body rejection. The existing 60-second production task was re-downloaded using --download-only without a generation request; metadata reports 60 seconds, 1280x720 and one audio track. Its bytes match the original file. Tests, Go build and archive parity are checked before publication. The website packages observed before release still lacked both long-video and download-only support; deployment parity must be checked after publishing.

## Consequences

Container metadata does not establish visual adherence or that audio is audible. Download-only requires the original credential. Website delivery may lag Git main until deployment; report the distinction rather than claiming parity without checking.
