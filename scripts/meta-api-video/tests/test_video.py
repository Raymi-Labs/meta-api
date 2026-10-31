import contextlib
import hashlib
import importlib.util
import io
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest.mock import patch

SCRIPT = Path(__file__).resolve().parents[1] / "scripts/meta_api_video.py"
spec = importlib.util.spec_from_file_location("video", SCRIPT)
video = importlib.util.module_from_spec(spec)
spec.loader.exec_module(video)
KEY = "fixture-secret-never-print"


class VideoTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.state = Path(self.tmp.name) / "task.json"
        self.environment = patch.dict(os.environ, {"META_API_KEY": KEY})
        self.environment.start()
        self.addCleanup(self.environment.stop)
        self.stdout = io.StringIO()
        self.output = contextlib.redirect_stdout(self.stdout)
        self.output.__enter__()
        self.addCleanup(self.output.__exit__, None, None, None)
        self.now = 0
        self.calls = []

    def args(self, *extra):
        return video.parser().parse_args(["--prompt", "private fixture prompt", "--state", str(self.state), "--wait", "8", *extra])

    def sleep(self, seconds):
        self.now += seconds

    def run_request(self, args, responses):
        answers = iter(responses)
        def request(origin, suffix, key, payload=None, timeout=30):
            self.calls.append((origin, suffix, key, payload))
            value = next(answers)
            if isinstance(value, Exception):
                raise value
            if payload is None and isinstance(value, dict):
                value = dict(value)
                value.setdefault("id", "task_fixture")
            return value
        return video.run(args, request=request, clock=lambda: self.now, sleep=self.sleep)

    def assert_private(self):
        content = self.state.read_text() + self.stdout.getvalue()
        self.assertNotIn(KEY, content)
        self.assertNotIn("private fixture prompt", self.state.read_text())

    def seed(self):
        video.save_state(self.state, {"version": 1, "origin": "https://meta-api.vip",
                         "task_id": "task_fixture", "phase": "queued", "model": video.MODEL,
                         "credential_fingerprint": hashlib.sha256(KEY.encode()).hexdigest()}, exclusive=True)

    def test_generation_polls_once_per_state_and_returns_url(self):
        code = self.run_request(self.args(), [{"id": "task_fixture"}, {"status": "queued"},
                    {"status": "in_progress"}, {"status": "completed", "metadata": {"url": "https://example.invalid/video.mp4"}}])
        self.assertEqual(code, 0)
        self.assertEqual(sum(call[3] is not None for call in self.calls), 1)
        self.assertEqual(self.calls[0][3]["duration"], 1)
        self.assertEqual(self.calls[0][3]["resolution"], "480p")
        self.assertTrue(all(c[1] == "/v1/videos/task_fixture" for c in self.calls[1:]))
        self.assertEqual(json.loads(self.state.read_text())["phase"], "completed")
        self.assert_private()
        if os.name != "nt":
            self.assertEqual(self.state.stat().st_mode & 0o777, 0o600)

    def test_resume_queries_original_task_without_post(self):
        self.seed()
        self.assertEqual(self.run_request(self.args("--resume"), [{"status": "completed", "metadata": {"url": "https://example.invalid/video.mp4"}}]), 0)
        self.assertIsNone(self.calls[0][3])
        self.assert_private()

    def test_uncertain_submission_is_never_retried(self):
        with self.assertRaises(video.VideoError):
            self.run_request(self.args(), [video.VideoError("connection_failed:TimeoutError")])
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(json.loads(self.state.read_text())["phase"], "submission_unknown")
        with self.assertRaises(video.VideoError):
            self.run_request(self.args("--resume"), [])
        with self.assertRaises(video.VideoError):
            self.run_request(self.args(), [])
        self.assertEqual(len(self.calls), 1)
        self.assert_private()

    def test_http_submission_rejection_does_not_retry(self):
        with self.assertRaises(video.HTTPFailure):
            self.run_request(self.args(), [video.HTTPFailure(400)])
        self.assertEqual(len(self.calls), 1)
        self.assertTrue(self.state.exists())

    def test_query_transient_errors_retry_only_get(self):
        self.seed()
        self.run_request(self.args("--resume"), [video.HTTPFailure(429), video.VideoError("connection_failed"),
                         {"status": "completed", "metadata": {"url": "https://example.invalid/v.mp4"}}])
        self.assertTrue(all(c[3] is None for c in self.calls))

    def test_query_forbidden_stops(self):
        self.seed()
        with self.assertRaises(video.HTTPFailure):
            self.run_request(self.args("--resume"), [video.HTTPFailure(403)])
        self.assertEqual(len(self.calls), 1)

    def test_pending_preserves_id(self):
        self.seed()
        code = self.run_request(self.args("--resume", "--wait", "1"), [{"status": "queued"}])
        self.assertEqual(code, 2)
        self.assertEqual(json.loads(self.state.read_text())["task_id"], "task_fixture")

    def test_terminal_failure_does_not_submit(self):
        self.seed()
        with self.assertRaises(video.VideoError):
            self.run_request(self.args("--resume"), [{"status": "failed"}])
        self.assertEqual(json.loads(self.state.read_text())["phase"], "failed")
        self.assertEqual(len(self.calls), 1)

    def test_false_completion_and_unknown_status_rejected(self):
        self.seed()
        for response in [{"status": "completed"}, {"status": "mystery"},
                         {"status": "completed", "metadata": {"url": "file:///tmp/video"}},
                         {"status": "completed", "metadata": {"url": "https://user:password@example.invalid/v"}}]:
            with self.subTest(response=response), self.assertRaises(video.VideoError):
                self.run_request(self.args("--resume"), [response])
        self.assertNotIn('"phase": "completed"', self.state.read_text())

    def test_mismatched_task_response_is_not_delivered(self):
        self.seed()
        with self.assertRaises(video.VideoError):
            self.run_request(self.args("--resume"), [{"id": "task_other", "status": "completed", "metadata": {"url": "https://example.invalid/v.mp4"}}])
        self.assertNotIn('"phase": "completed"', self.state.read_text())

    def test_missing_id_after_submit_preserves_uncertainty(self):
        with self.assertRaises(video.VideoError):
            self.run_request(self.args(), [{"status": "queued"}])
        self.assertEqual(len(self.calls), 1)
        self.assertEqual(json.loads(self.state.read_text())["phase"], "submission_unknown")

    def test_resume_cannot_switch_origin_or_key(self):
        self.seed()
        with self.assertRaises(video.VideoError):
            self.run_request(self.args("--resume", "--base-url", "https://cn.meta-api.vip"), [])
        with patch.dict(os.environ, {"META_API_KEY": "another-key"}), self.assertRaises(video.VideoError):
            self.run_request(self.args("--resume"), [])
        self.assertEqual(self.calls, [])

    def test_dry_run_needs_no_key_and_writes_nothing(self):
        with patch.dict(os.environ, {}, clear=True):
            self.assertEqual(self.run_request(self.args("--dry-run"), []), 0)
        self.assertFalse(self.state.exists())
        self.assertEqual(self.calls, [])

    def test_video_15_preserves_explicit_model(self):
        args = self.args("--model", "grok-imagine-video-1.5", "--dry-run")
        self.assertEqual(video.payload_for(args)["model"], "grok-imagine-video-1.5")
        self.assertEqual(self.run_request(args, []), 0)
        self.assertEqual(self.calls, [])
        self.assertFalse(self.state.exists())

    def test_1080p_requires_explicit_video_15_model(self):
        args = self.args("--model", "grok-imagine-video-1.5", "--resolution", "1080p", "--dry-run")
        self.assertEqual(video.payload_for(args)["resolution"], "1080p")
        self.assertEqual(self.run_request(args, []), 0)
        with self.assertRaises(video.VideoError):
            self.run_request(self.args("--resolution", "1080p"), [])
        self.assertEqual(self.calls, [])
        self.assertFalse(self.state.exists())

    def test_compatible_video_profiles_use_seconds_and_size(self):
        for name, resolution, size in [("flow-video-gemini-omni-flash-1.1", "720p", "1280x720"), ("flow-video-gemini-omni-flash-1.1-1080p", "1080p", "1920x1080"), ("muse-video", "720p", "1280x720"), ("snow-grok-imagine-video", "720p", "1280x720")]:
            args = self.args("--model", name)
            payload = video.payload_for(args)
            self.assertEqual(payload["seconds"], "4")
            self.assertEqual(payload["size"], size)
            self.assertEqual(args.resolution, resolution)
            self.assertNotIn("duration", payload)
        with self.assertRaises(video.VideoError):
            video.payload_for(self.args("--model", "muse-video", "--duration", "31"))

    def test_muse_declared_range_preserves_request_and_rejects_invalid_specs(self):
        for duration in (4, 5, 8, 16, 30):
            args = self.args("--model", "muse-video", "--duration", str(duration))
            payload = video.payload_for(args)
            self.assertEqual(payload["seconds"], str(duration))
            self.assertEqual(payload["size"], "1280x720")
        portrait = video.payload_for(self.args("--model", "muse-video", "--duration", "8", "--aspect-ratio", "9:16"))
        self.assertEqual(portrait["size"], "720x1280")
        for options in (("--duration", "3"), ("--duration", "31"), ("--duration", "0"),
                        ("--resolution", "1080p"), ("--resolution", "480p"), ("--aspect-ratio", "1:1")):
            with self.subTest(options=options), self.assertRaises(video.VideoError):
                self.run_request(self.args("--model", "muse-video", *options), [])
        self.assertEqual(self.calls, [])
        self.assertFalse(self.state.exists())

    def test_muse_long_request_single_submit_and_no_actual_duration_claim(self):
        args = self.args("--model", "muse-video", "--duration", "30")
        code = self.run_request(args, [{"id": "task_fixture"}, {"status": "completed"}])
        self.assertEqual(code, 0)
        posts = [call for call in self.calls if call[1] == "/v1/videos"]
        self.assertEqual(len(posts), 1)
        saved = json.loads(self.state.read_text())
        self.assertEqual(saved["duration"], 30)
        self.assertNotIn("actual_duration", saved)

    def test_veo_lite_requires_local_png_or_jpeg(self):
        args = self.args("--model", "flow-video-veo-3.1-lite")
        with self.assertRaises(video.VideoError):
            video.payload_for(args)
        image = Path(self.tmp.name) / "reference.png"
        image.write_bytes(b"\x89PNG\r\n\x1a\nfixture")
        payload = video.payload_for(self.args("--model", "flow-video-veo-3.1-lite", "--image", str(image)))
        self.assertEqual(payload["seconds"], "8")
        self.assertTrue(payload["input_reference"].startswith("data:image/png;base64,"))
        image.write_text("not an image")
        with self.assertRaises(video.VideoError):
            video.payload_for(self.args("--model", "flow-video-veo-3.1-lite", "--image", str(image)))

    def test_muse_completion_exposes_only_same_origin_content_endpoint(self):
        self.seed()
        state = json.loads(self.state.read_text())
        state["model"] = "muse-video"
        video.save_state(self.state, state)
        code = self.run_request(self.args("--resume"), [{"status": "completed"}])
        self.assertEqual(code, 0)
        saved = json.loads(self.state.read_text())
        self.assertEqual(saved["url"], "https://meta-api.vip/v1/videos/task_fixture/content")
        self.assertTrue(saved["authenticated_content"])
        self.assert_private()

    def test_reference_payloads_are_not_text_only(self):
        image = Path(self.tmp.name) / "reference.png"
        image.write_bytes(b"\x89PNG\r\n\x1a\nfixture-distinctive-image")
        for model in ("muse-video", "grok-imagine-video", "grok-imagine-video-1.5"):
            with self.subTest(model=model):
                payload = video.payload_for(self.args("--model", model, "--image", str(image)))
                encoded = payload["input_reference"] if model == "muse-video" else payload["image"]["url"]
                self.assertTrue(encoded.startswith("data:image/png;base64,"))
                self.assertNotIn(str(image), json.dumps(payload))
                if model == "muse-video":
                    self.assertEqual(payload["seconds"], "4")
                else:
                    self.assertNotIn("input_reference", payload)

    def test_reference_dry_run_does_not_disclose_image_bytes(self):
        image = Path(self.tmp.name) / "reference.jpg"
        image.write_bytes(b"\xff\xd8\xffprivate-fixture-image")
        for model in ("muse-video", "grok-imagine-video"):
            payload = video.payload_for(self.args("--model", model, "--image", str(image)))
            encoded = payload.get("input_reference") or payload["image"]["url"]
            self.run_request(self.args("--model", model, "--image", str(image), "--dry-run"), [])
            self.assertNotIn(encoded, self.stdout.getvalue())
        self.assertFalse(self.state.exists())
        self.assertEqual(self.calls, [])

    def test_reference_submit_once_resume_without_source_image(self):
        image = Path(self.tmp.name) / "reference.png"
        image.write_bytes(b"\x89PNG\r\n\x1a\nprivate-reference")
        args = self.args("--model", "muse-video", "--image", str(image))
        self.run_request(args, [{"id": "task_fixture"}, {"status": "completed"}])
        self.assertEqual(sum(call[3] is not None for call in self.calls), 1)
        data = self.state.read_text() + self.stdout.getvalue()
        self.assertNotIn("data:image/", data)
        self.assertNotIn(str(image), data)
        image.unlink()
        self.calls.clear()
        self.run_request(self.args("--resume"), [{"status": "completed"}])
        self.assertTrue(all(call[3] is None for call in self.calls))

    def test_bad_reference_rejected_before_submission(self):
        image = Path(self.tmp.name) / "reference.png"
        for data in (b"not-an-image", b"\x89PNG\r\n\x1a\n" + b"x" * (20 * 1024 * 1024)):
            image.write_bytes(data)
            with self.assertRaises(video.VideoError):
                self.run_request(self.args("--model", "muse-video", "--image", str(image)), [])
        image.unlink()
        with self.assertRaisesRegex(video.VideoError, "Cannot read reference"):
            self.run_request(self.args("--image", str(image)), [])
        self.assertFalse(self.state.exists())
        self.assertEqual(self.calls, [])

    def test_invalid_inputs_do_not_submit(self):
        for extra in [("--model", "sora-2"), ("--prompt", " "), ("--duration", "0"),
                      ("--duration", "16"), ("--base-url", "https://attacker.invalid"),
                      ("--base-url", "https://key@meta-api.vip"), ("--base-url", "https://meta-api.vip/?key=x")]:
            with self.subTest(extra=extra), self.assertRaises(video.VideoError):
                self.run_request(self.args(*extra), [])
        self.assertFalse(self.state.exists())

    def test_concurrent_starts_reserve_one_submission(self):
        entered, release = threading.Event(), threading.Event()
        errors = []
        def request(*args, **kwargs):
            entered.set()
            release.wait(2)
            raise video.VideoError("uncertain")
        def first():
            try:
                video.run(self.args(), request=request)
            except video.VideoError as exc:
                errors.append(str(exc))
        thread = threading.Thread(target=first)
        thread.start()
        try:
            self.assertTrue(entered.wait(2))
            with self.assertRaises(video.VideoError):
                self.run_request(self.args(), [])
        finally:
            release.set()
            thread.join(3)
        self.assertFalse(thread.is_alive())
        self.assertEqual(errors, ["uncertain"])
        self.assertEqual(self.calls, [])

    def test_existing_empty_file_and_symlink_are_not_overwritten(self):
        self.state.write_text("")
        with self.assertRaises(video.VideoError):
            self.run_request(self.args(), [])
        self.assertEqual(self.state.read_text(), "")
        self.assertEqual(self.calls, [])

    def test_network_errors_retain_safe_cause_and_redirects_are_refused(self):
        with patch.object(video.urllib.request, "build_opener") as opener:
            opener.return_value.open.side_effect = video.urllib.error.URLError(TimeoutError("https://secret@proxy.invalid"))
            with self.assertRaisesRegex(video.VideoError, "connection_failed:TimeoutError") as error:
                video.request_json("https://meta-api.vip", "/v1/videos", KEY)
            self.assertNotIn("secret", str(error.exception))
        self.assertIsNone(video.NoRedirect().redirect_request(None, None, 302, "", {}, "https://evil.invalid"))

    def test_real_json_http_transport(self):
        seen = []
        class Handler(BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
                seen.append((self.command, self.path, self.headers["Content-Type"], body))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"id":"task_http"}')
            def do_GET(self):
                seen.append((self.command, self.path))
                self.send_response(200)
                self.end_headers()
                self.wfile.write(b'{"id":"task_http","status":"completed","metadata":{"url":"https://example.invalid/v.mp4"}}')
            def log_message(self, *args):
                pass
        server = ThreadingHTTPServer(("127.0.0.1", 0), Handler)
        thread = threading.Thread(target=server.serve_forever)
        thread.start()
        try:
            with patch.object(video, "normalize_origin", return_value="http://127.0.0.1:"+str(server.server_port)):
                self.assertEqual(video.run(self.args()), 0)
            self.assertEqual(seen[0][0:3], ("POST", "/v1/videos", "application/json"))
            self.assertEqual(seen[1], ("GET", "/v1/videos/task_http"))
        finally:
            server.shutdown()
            server.server_close()
            thread.join(3)
        self.assertFalse(thread.is_alive())
        self.assert_private()

    def test_archive_runs_without_source_tree(self):
        root = SCRIPT.parents[3]
        archive_path = root / "web/public/downloads/meta-api-video-skill.zip"
        target = Path(self.tmp.name) / "unpacked"
        with zipfile.ZipFile(archive_path) as archive:
            self.assertEqual(sorted(archive.namelist()), ["meta-api-video/SKILL.md", "meta-api-video/scripts/meta_api_video.py"])
            archive.extractall(target)
        result = subprocess.run([sys.executable, str(target / "meta-api-video/scripts/meta_api_video.py"),
                                 "--dry-run", "--prompt", "ocean", "--state", str(self.state)],
                                capture_output=True, text=True, timeout=5,
                                env={k: v for k, v in os.environ.items() if k != "META_API_KEY"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(json.loads(result.stdout)["network_requests"], 0)
        self.assertFalse(self.state.exists())

    def test_parent_timeout_terminates_and_reaps_worker(self):
        # This wrapper injects a hanging local fixture into the real supervisor and child.
        wrapper = Path(self.tmp.name) / "supervisor_fixture.py"
        marker = Path(self.tmp.name) / "pid"
        wrapper.write_text("import importlib.util, pathlib, os, time\n"
            + "s=importlib.util.spec_from_file_location('video',"+repr(str(SCRIPT))+");m=importlib.util.module_from_spec(s);s.loader.exec_module(m)\n"
            + "m.__file__=__file__\n"
            + "def hang(args):\n pathlib.Path("+repr(str(marker))+").write_text(str(os.getpid()))\n time.sleep(90)\n"
            + "m.run=hang\nraise SystemExit(m.main())\n")
        result = subprocess.run([sys.executable, str(wrapper), "--wait", "1", "--state", str(self.state)],
                                capture_output=True, text=True, timeout=12)
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertIn("interrupted", result.stdout)
        pid = int(marker.read_text())
        with self.assertRaises(ProcessLookupError):
            os.kill(pid, 0)


if __name__ == "__main__":
    unittest.main()
