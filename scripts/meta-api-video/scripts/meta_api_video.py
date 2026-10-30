#!/usr/bin/env python3
"""One paid JSON submission, resumable polling, and no credential persistence."""
import argparse
import base64
import hashlib
import json
import os
from pathlib import Path
import re
import signal
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request

HOSTS = {"meta-api.vip", "cn.meta-api.vip", "nan.meta-api.vip"}
MODEL = "grok-imagine-video"
MODELS = {MODEL, "grok-imagine-video-1.5"}
COMPAT_PROFILES = {
    "flow-video-gemini-omni-flash-1.1": (4, "720p", False),
    "flow-video-gemini-omni-flash-1.1-1080p": (4, "1080p", False),
    "muse-video": (4, "720p", False),
    "flow-video-veo-3.1-lite": (8, "720p", True),
    "snow-grok-imagine-video": (4, "720p", False),
}
MODELS.update(COMPAT_PROFILES)

MAX_JSON = 1024 * 1024


class VideoError(Exception):
    pass


class HTTPFailure(VideoError):
    def __init__(self, status):
        self.status = status
        super().__init__("http_" + str(status))


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def emit(**fields):
    print(json.dumps(fields, ensure_ascii=False), flush=True)


def normalize_origin(value):
    parsed = urllib.parse.urlsplit(value.rstrip("/"))
    if (parsed.scheme != "https" or parsed.hostname not in HOSTS or
            parsed.username or parsed.password or parsed.query or parsed.fragment or
            parsed.port not in (None, 443) or parsed.path not in ("", "/v1")):
        raise VideoError("Use a YuanHeng HTTPS base URL, optionally ending in /v1.")
    return "https://" + parsed.hostname


def task_id(value):
    if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_-]{1,160}", value):
        raise VideoError("Invalid or missing task ID; do not resubmit. Check server task logs.")
    return value


def load_state(path):
    if path.is_symlink():
        raise VideoError("Recovery file must not be a symlink.")
    with path.open("rb") as stream:
        data = stream.read(MAX_JSON + 1)
    if len(data) > MAX_JSON:
        raise VideoError("Recovery file is too large.")
    state = json.loads(data)
    if not isinstance(state, dict) or state.get("version") != 1:
        raise VideoError("Unrecognized recovery record.")
    normalize_origin(state["origin"])
    return state


def save_state(path, state, exclusive=False):
    path.parent.mkdir(parents=True, exist_ok=True)
    data = json.dumps(state, ensure_ascii=False, indent=2).encode()
    if exclusive:
        try:
            fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
        except FileExistsError:
            raise VideoError("Recovery file already exists. Use --resume; do not submit again.") from None
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        return
    fd, temporary = tempfile.mkstemp(prefix=".video-state-", dir=path.parent)
    try:
        with os.fdopen(fd, "wb") as stream:
            stream.write(data)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)


def request_json(origin, suffix, key, payload=None, timeout=30):
    data = None if payload is None else json.dumps(payload).encode()
    request = urllib.request.Request(origin + suffix, data=data, headers={
        "Authorization": "Bearer " + key, "Content-Type": "application/json",
        "Accept": "application/json", "User-Agent": "yuanheng-video-skill/1.0",
    })
    opener = urllib.request.build_opener(NoRedirect())
    try:
        with opener.open(request, timeout=timeout) as response:
            raw = response.read(MAX_JSON + 1)
    except urllib.error.HTTPError as exc:
        status = exc.code
        exc.close()
        raise HTTPFailure(status) from None
    except (urllib.error.URLError, TimeoutError, OSError) as exc:
        cause = getattr(exc, "reason", exc)
        # Do not echo proxy URLs, headers, keys, or a server-controlled response.
        raise VideoError("connection_failed:" + type(cause).__name__) from None
    if len(raw) > MAX_JSON:
        raise VideoError("Response exceeds the JSON limit.")
    try:
        body = json.loads(raw)
    except (ValueError, UnicodeError):
        raise VideoError("Invalid JSON response.") from None
    if not isinstance(body, dict):
        raise VideoError("Expected a JSON object response.")
    return body


def payload_for(args):
    if args.model not in MODELS:
        raise VideoError("Unsupported video model; use a verified profile.")
    if not args.prompt or not args.prompt.strip():
        raise VideoError("A non-empty prompt is required.")
    profile = COMPAT_PROFILES.get(args.model)
    args.duration = args.duration if args.duration is not None else (profile[0] if profile else 1)
    args.resolution = args.resolution or (profile[1] if profile else "480p")
    if profile:
        if args.duration != profile[0] or args.resolution != profile[1]:
            raise VideoError("This model profile only supports the verified duration and resolution.")
        if bool(args.image) != profile[2]:
            raise VideoError("This profile requires --image." if profile[2] else "This profile does not accept reference images.")
        if args.aspect_ratio not in {"16:9", "9:16"}:
            raise VideoError("This profile requires 16:9 or 9:16.")
        width, height = (1920, 1080) if args.resolution == "1080p" else (1280, 720)
        if args.aspect_ratio == "9:16":
            width, height = height, width
        payload = {"model": args.model, "prompt": args.prompt.strip(), "seconds": str(args.duration), "size": f"{width}x{height}"}
        if args.image:
            with Path(args.image).expanduser().open("rb") as stream:
                raw = stream.read(20 * 1024 * 1024 + 1)
            if len(raw) > 20 * 1024 * 1024:
                raise VideoError("Reference image exceeds 20 MiB.")
            if raw.startswith(b"\x89PNG\r\n\x1a\n"):
                mime = "image/png"
            elif raw.startswith(b"\xff\xd8\xff"):
                mime = "image/jpeg"
            else:
                raise VideoError("Reference must be a PNG or JPEG image.")
            payload["input_reference"] = "data:" + mime + ";base64," + base64.b64encode(raw).decode("ascii")
        return payload
    if args.image:
        raise VideoError("Grok profiles currently support text-to-video only.")
    if args.resolution == "1080p" and args.model != "grok-imagine-video-1.5":
        raise VideoError("1080p requires grok-imagine-video-1.5.")
    if not 1 <= args.duration <= 15:
        raise VideoError("Duration must be between 1 and 15 seconds; provider limits still apply.")
    return {"model": args.model, "prompt": args.prompt.strip(), "duration": args.duration,
            "aspect_ratio": args.aspect_ratio, "resolution": args.resolution}


def run(args, request=request_json, clock=time.monotonic, sleep=time.sleep):
    path = Path(args.state).expanduser()
    if args.resume:
        state = load_state(path)
        origin = normalize_origin(state["origin"])
        if args.base_url and normalize_origin(args.base_url) != origin:
            raise VideoError("Resume must use the original API origin.")
        identifier = task_id(state.get("task_id"))
    else:
        origin = normalize_origin(args.base_url or "https://meta-api.vip")
        payload = payload_for(args)
        state = {"version": 1, "origin": origin, "model": args.model,
                 "phase": "submission_unknown", "duration": args.duration,
                 "resolution": args.resolution, "aspect_ratio": args.aspect_ratio}
    if args.dry_run:
        emit(mode="resume" if args.resume else "generate", origin=origin,
             model=state["model"], state_file=str(path), network_requests=0,
             **({"task_id": identifier} if args.resume else {"parameters": {k: ("[local reference image]" if k == "input_reference" else v) for k, v in payload.items()}}))
        return 0
    key = os.environ.get("META_API_KEY", "").strip()
    if not key or len(key) > 8192 or any(not 33 <= ord(c) <= 126 for c in key):
        raise VideoError("Set META_API_KEY to the user's YuanHeng API key.")
    fingerprint = hashlib.sha256(key.encode()).hexdigest()
    if args.resume and state.get("credential_fingerprint") != fingerprint:
        raise VideoError("Resume requires the original credential. No alternate key will be tried.")
    deadline = clock() + args.wait
    if not args.resume:
        state["credential_fingerprint"] = fingerprint
        save_state(path, state, exclusive=True)
        emit(phase="submitting", state_file=str(path), paid_submissions=1)
        try:
            response = request(origin, "/v1/videos", key, payload=payload,
                               timeout=min(600, args.wait))
            identifier = task_id(response.get("id"))
        except VideoError:
            emit(phase="submission_unknown", state_file=str(path),
                 action="Check task and usage logs before any new submission.")
            raise
        # Emit before disk I/O so an ID remains visible even if persistence fails.
        emit(phase="accepted", task_id=identifier)
        state.update(task_id=identifier, phase="queued")
        save_state(path, state)
    interval = 2
    while clock() < deadline:
        try:
            response = request(origin, "/v1/videos/" + identifier, key,
                               timeout=max(0.1, min(30, deadline - clock())))
        except HTTPFailure as exc:
            if exc.status != 429 and not 500 <= exc.status <= 599:
                raise
            emit(phase="query_retry", task_id=identifier, error=str(exc))
        except VideoError as exc:
            emit(phase="query_retry", task_id=identifier, error=str(exc))
        else:
            if response.get("id") != identifier:
                raise VideoError("Task response ID does not match the requested task.")
            status = response.get("status")
            if status not in {"queued", "in_progress", "completed", "failed"}:
                raise VideoError("Unknown task status; keep the task ID and inspect logs.")
            if status == "completed":
                metadata = response.get("metadata")
                url = metadata.get("url") if isinstance(metadata, dict) else None
                if state.get("model") in COMPAT_PROFILES:
                    url = url or response.get("url") or response.get("video_url")
                    if not url:
                        url = origin + "/v1/videos/" + identifier + "/content"
                        state.update(phase="completed", url=url, authenticated_content=True)
                        save_state(path, state)
                        emit(phase="completed", task_id=identifier, content_url=url,
                             requires_authorization=True, state_file=str(path))
                        return 0
                parsed = urllib.parse.urlsplit(url) if isinstance(url, str) else None
                if (not parsed or parsed.scheme != "https" or not parsed.hostname or
                        parsed.username or parsed.password or any(ord(c) < 32 for c in url)):
                    raise VideoError("Completed task has no valid HTTPS video URL; keep the task ID.")
                state.update(phase="completed", url=url)
                save_state(path, state)
                emit(phase="completed", task_id=identifier, video_url=url, state_file=str(path))
                return 0
            state["phase"] = status
            save_state(path, state)
            emit(phase=status, task_id=identifier)
            if status == "failed":
                raise VideoError("Video generation failed. Check task/usage logs; no resubmission was made.")
        remaining = deadline - clock()
        if remaining <= 0:
            break
        sleep(min(interval, remaining))
        interval = min(interval * 1.5, 15)
    emit(phase="pending", task_id=identifier, state_file=str(path), action="Use --resume to query this task.")
    return 2


def parser():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument("--base-url", help="YuanHeng API origin, optional /v1 suffix")
    p.add_argument("--model", default=MODEL)
    p.add_argument("--prompt")
    p.add_argument("--image", help="PNG/JPEG reference, only for the verified Veo Lite profile")
    p.add_argument("--duration", type=int, default=None)
    p.add_argument("--aspect-ratio", choices=["16:9", "9:16", "1:1"], default="16:9")
    p.add_argument("--resolution", choices=["480p", "720p", "1080p"], default=None)
    p.add_argument("--state", default="output/video/task.json")
    p.add_argument("--resume", action="store_true")
    p.add_argument("--dry-run", action="store_true")
    p.add_argument("--wait", type=int, default=420, help="Maximum wait seconds (1-3600); pending tasks can be resumed")
    p.add_argument("--worker", action="store_true", help=argparse.SUPPRESS)
    return p


def main():
    args = parser().parse_args()
    if not 1 <= args.wait <= 3600:
        emit(error="--wait must be between 1 and 3600 seconds")
        return 1
    if not args.worker and not args.dry_run:
        def interrupted(signum, frame):
            raise KeyboardInterrupt
        signal.signal(signal.SIGTERM, interrupted)
        child = subprocess.Popen([sys.executable, str(Path(__file__).resolve()), *sys.argv[1:], "--worker"])
        try:
            return child.wait(timeout=args.wait + 5)
        except (subprocess.TimeoutExpired, KeyboardInterrupt):
            emit(phase="interrupted", state_file=args.state,
                 action="Do not resubmit. Resume if the record contains a task ID; otherwise check server logs.")
            return 2
        finally:
            if child.poll() is None:
                child.terminate()
                try:
                    child.wait(timeout=3)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()
    try:
        return run(args)
    except (VideoError, OSError, ValueError, KeyError, TypeError) as exc:
        message = str(exc) if isinstance(exc, VideoError) else "Local recovery/validation error: " + type(exc).__name__
        emit(error=message, state_file=args.state)
        return 1


if __name__ == "__main__":
    sys.exit(main())
