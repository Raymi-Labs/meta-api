import datetime as dt
import importlib.util
import json
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location("retain", Path(__file__).with_name("image-retain.py"))
retain = importlib.util.module_from_spec(spec)
spec.loader.exec_module(retain)
NOW = dt.datetime(2026, 10, 17, tzinfo=dt.timezone.utc)


def image(name, age=60, tags=None):
    return {"Id": "sha256:" + name, "Created": (NOW - dt.timedelta(days=age)).isoformat(),
            "RepoTags": tags if tags is not None else [retain.REPOSITORY + ":" + name],
            "RepoDigests": []}


class RetentionTests(unittest.TestCase):
    def test_preserves_recent_boundary_and_all_protected_references(self):
        images = [image("old"), image("current"), image("rollback"), image("recent", 29), image("boundary", 30)]
        selected = retain.candidates(images, {"sha256:current", retain.REPOSITORY + ":rollback"}, NOW)
        self.assertEqual([i["Id"] for i in selected], ["sha256:old"])

    def test_excludes_other_repositories_shared_tags_and_unknown_dangling(self):
        images = [image("mysql", tags=["mysql:8.0"]), image("unknown", tags=[]),
                  image("shared", tags=[retain.REPOSITORY + ":old", "other:keep"]),
                  image("similar", tags=[retain.REPOSITORY + "-other:old"]), image("digest", tags=[])]
        images[-1]["RepoDigests"] = [retain.REPOSITORY + "@sha256:known"]
        self.assertEqual([i["Id"] for i in retain.candidates(images, set(), NOW)], ["sha256:digest"])

    def test_preview_never_deletes_and_apply_never_forces(self):
        with patch.object(retain.subprocess, "run") as execute:
            retain.execute([image("old")], False)
            execute.assert_not_called()
            retain.execute([image("old")], True)
            execute.assert_called_once_with(["docker", "image", "rm", "sha256:old"], check=True)

    def test_delete_failure_stops_remaining_deletions(self):
        with patch.object(retain.subprocess, "run", side_effect=subprocess.CalledProcessError(1, "docker")) as execute:
            with self.assertRaises(subprocess.CalledProcessError):
                retain.execute([image("first"), image("second")], True)
            self.assertEqual(execute.call_count, 1)

    def test_reference_inventory_and_failure_guards(self):
        with tempfile.TemporaryDirectory() as root:
            app = Path(root)
            (app / ".release-manager").mkdir()
            state = app / ".release-manager/state.env"
            state.write_text("CURRENT_IMAGE=" + retain.REPOSITORY + ":current\nCURRENT_ID=sha256:current\n"
                             "PREVIOUS_IMAGE=" + retain.REPOSITORY + ":previous\nPREVIOUS_ID=sha256:previous\n")
            backup = app / "backups/one-click/20260901-010101-backup"
            backup.mkdir(parents=True)
            (backup / "state.env").write_text("PREVIOUS_ID=sha256:backup-previous\n")
            compose = backup / "docker-compose.yml"
            compose.write_text("services:\n  new-api:\n    image: '" + retain.REPOSITORY + ":backup' # retained\n")
            images = [image(name) for name in ("current", "previous", "container", "compose", "backup", "backup-previous", "old")]

            def docker(*args):
                if args[:3] == ("docker", "ps", "-aq"):
                    return "running\nstopped"
                if args[:2] == ("docker", "inspect"):
                    return json.dumps([{"Image": "sha256:container"}])
                if args[:2] == ("docker", "compose"):
                    return retain.REPOSITORY + ":compose"
                raise AssertionError(args)

            with patch.object(retain, "APP", app), patch.object(retain, "run", side_effect=docker):
                protected = retain.protected_references(images)
                self.assertEqual([i["Id"] for i in retain.candidates(images, protected, NOW)], ["sha256:old"])
                with self.assertRaisesRegex(ValueError, "image is missing"):
                    retain.protected_references(images[1:])
                compose.write_text("image: ${NEWAPI_IMAGE}\n")
                with self.assertRaisesRegex(ValueError, "unresolved backup image"):
                    retain.protected_references(images)
                compose.write_text("image: mysql:8.0\n")
                (backup.parent / "unexpected").symlink_to(backup, target_is_directory=True)
                with self.assertRaisesRegex(ValueError, "unexpected entry"):
                    retain.protected_references(images)
            state.write_text("CURRENT_IMAGE=one\n")
            with self.assertRaisesRegex(ValueError, "incomplete"):
                retain.read_state(state, required=True)

    def test_lock_contention_exits_before_docker(self):
        with patch.object(retain.argparse.ArgumentParser, "parse_args", return_value=type("Args", (), {"apply": True})()), \
                patch("builtins.open", unittest.mock.mock_open()), \
                patch.object(retain.fcntl, "flock", side_effect=BlockingIOError), \
                patch.object(retain, "inspect_images") as inspect:
            with self.assertRaises(BlockingIOError):
                retain.main()
            inspect.assert_not_called()


if __name__ == "__main__":
    unittest.main()
