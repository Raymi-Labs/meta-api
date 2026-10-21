import importlib.util
import json
import subprocess
import unittest
from pathlib import Path


spec = importlib.util.spec_from_file_location("optional_release", Path(__file__).with_name("plan-optional.py"))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

TAG = "v1.0.0-compat.53"
SHA = "a" * 40


class OptionalReleaseTests(unittest.TestCase):
    def reader(self, *, version=TAG, draft=False, assets=None, revision=SHA, linux_complete=True):
        self.commands = []
        release_assets = list(assets or [])
        if linux_complete:
            release_assets.extend({"name": name, "state": "uploaded", "size": 100} for name in (f"new-api-{TAG}", f"new-api-arm64-{TAG}"))

        def read(args):
            self.commands.append(args)
            if args[:2] == ["git", "fetch"]:
                return ""
            if args[:2] == ["git", "rev-parse"]:
                return revision
            if args[:2] == ["git", "show"]:
                return version + "\n"
            if args[:3] == ["gh", "release", "view"]:
                return json.dumps({"isDraft": draft, "assets": release_assets})
            raise AssertionError(args)

        return read

    def test_missing_asset_pins_exact_tag_commit(self):
        result = module.plan(TAG, "windows", "owner/repo", self.reader())
        self.assertEqual(result, {"revision": SHA, "asset": f"new-api-{TAG}.exe", "runner": "windows-latest", "build_needed": "true"})
        self.assertIn(["git", "fetch", "--no-tags", "--depth=1", "origin", f"refs/tags/{TAG}"], self.commands)
        self.assertIn(["git", "show", f"{SHA}:VERSION"], self.commands)

    def test_macos_selects_only_macos_runner(self):
        result = module.plan(TAG, "macos", "owner/repo", self.reader())
        self.assertEqual(result["runner"], "macos-latest")
        self.assertEqual(result["asset"], f"new-api-macos-{TAG}")

    def test_existing_asset_avoids_allocating_runner(self):
        asset = {"name": f"new-api-{TAG}.exe", "state": "uploaded", "size": 100}
        result = module.plan(TAG, "windows", "owner/repo", self.reader(assets=[asset]))
        self.assertEqual(result["build_needed"], "false")

    def test_other_platform_asset_does_not_skip_requested_build(self):
        asset = {"name": f"new-api-macos-{TAG}", "state": "uploaded", "size": 100}
        result = module.plan(TAG, "windows", "owner/repo", self.reader(assets=[asset]))
        self.assertEqual(result["build_needed"], "true")

    def test_invalid_inputs_rejected_before_external_commands(self):
        for tag, platform in [("main", "windows"), (TAG + "\nrunner=macos-latest", "windows"), ("--help", "windows"), (TAG, "linux")]:
            with self.subTest(tag=tag, platform=platform):
                read = self.reader()
                with self.assertRaises(ValueError):
                    module.plan(tag, platform, "owner/repo", read)
                self.assertEqual(self.commands, [])

    def test_wrong_revision_version_and_draft_rejected(self):
        for values in [{"revision": "main"}, {"version": "v0.0.0"}, {"draft": True}]:
            with self.subTest(values=values), self.assertRaises(ValueError):
                module.plan(TAG, "windows", "owner/repo", self.reader(**values))

    def test_incomplete_assets_not_overwritten(self):
        for state, size in [("starter", 100), ("uploaded", 0)]:
            asset = {"name": f"new-api-{TAG}.exe", "state": state, "size": size}
            with self.subTest(state=state, size=size), self.assertRaises(ValueError):
                module.plan(TAG, "windows", "owner/repo", self.reader(assets=[asset]))

    def test_network_failure_is_not_treated_as_missing_asset(self):
        read = self.reader()
        def fail(args):
            if args[0] == "gh":
                raise subprocess.CalledProcessError(1, args)
            return read(args)
        with self.assertRaises(subprocess.CalledProcessError):
            module.plan(TAG, "windows", "owner/repo", fail)

    def test_incomplete_linux_release_does_not_allocate_optional_runner(self):
        with self.assertRaises(ValueError):
            module.plan(TAG, "windows", "owner/repo", self.reader(linux_complete=False))


if __name__ == "__main__":
    unittest.main()
