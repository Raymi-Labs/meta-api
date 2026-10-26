"""Verify real systemd boot ordering in a disposable root, without a manager."""
import configparser
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest


HERE = Path(__file__).resolve().parent
SOCKETS = {"pulse-logdb.socket": ("10.77.0.1:3306", "yes"),
           "pulse-newapi-api.socket": ("10.77.0.1:3000", "no")}


class SocketContractTests(unittest.TestCase):
    def test_private_listeners_and_activation_modes_are_preserved(self):
        for name, (address, accepted) in SOCKETS.items():
            parser = configparser.ConfigParser(interpolation=None)
            parser.read(HERE / name)
            with self.subTest(socket=name):
                self.assertEqual(parser["Socket"]["ListenStream"], address)
                self.assertEqual(parser["Socket"]["Accept"], accepted)
                self.assertEqual(parser["Unit"]["DefaultDependencies"], "no")
                self.assertTrue({"wg-quick@wg0.service", "docker.service"}.issubset(parser["Unit"]["After"].split()))
                self.assertIn("wg-quick@wg0.service", parser["Unit"]["Requires"].split())
                self.assertIn("multi-user.target", parser["Unit"]["Before"].split())
                self.assertIn("shutdown.target", parser["Unit"]["Before"].split())
                self.assertIn("shutdown.target", parser["Unit"]["Conflicts"].split())
                self.assertEqual(parser["Install"]["WantedBy"], "multi-user.target")


@unittest.skipUnless(sys.platform.startswith("linux") and shutil.which("systemd-analyze"),
                     "requires Linux and systemd-analyze")
class SystemdBootGraphTests(unittest.TestCase):
    def verify_graph(self, legacy=False, only_install_target=False):
        with tempfile.TemporaryDirectory(prefix="newapi-socket-boot-") as directory:
            root = Path(directory)
            units = root / "etc/systemd/system"
            units.mkdir(parents=True)
            binary = root / "usr/bin/true"
            binary.parent.mkdir(parents=True)
            shutil.copyfile(shutil.which("true"), binary)
            binary.chmod(0o755)
            for name in ("sysinit", "sockets", "shutdown"):
                (units / (name + ".target")).write_text("[Unit]\nDefaultDependencies=no\n")
            (units / "basic.target").write_text(
                "[Unit]\nDefaultDependencies=no\nRequires=sockets.target sysinit.target\n"
                "After=sockets.target sysinit.target\n")
            (units / "multi-user.target").write_text(
                "[Unit]\nDefaultDependencies=no\nRequires=basic.target\nAfter=basic.target\n")
            for name in ("docker.service", "wg-quick@wg0.service"):
                (units / name).write_text("[Service]\nType=oneshot\nExecStart=/usr/bin/true\nRemainAfterExit=yes\n")
            # Real services get their normal implicit After=basic.target;
            # replacing only their executable keeps the dependency graph real.
            (units / "pulse-logdb@.service").write_text(
                "[Unit]\nRequires=docker.service\nAfter=docker.service\n"
                "[Service]\nExecStart=/usr/bin/true\nStandardInput=socket\n")
            (units / "pulse-newapi-api.service").write_text(
                "[Unit]\nRequires=pulse-newapi-api.socket\nAfter=docker.service\n"
                "[Service]\nExecStart=/usr/bin/true\n")
            for name in SOCKETS:
                source = (HERE / name).read_text()
                if legacy:
                    source = source.replace("DefaultDependencies=no\n", "")
                    source = source.replace("Before=multi-user.target shutdown.target\n", "")
                    source = source.replace("Conflicts=shutdown.target\n", "")
                target = "sockets.target" if legacy and not only_install_target else "multi-user.target"
                source = source.replace("WantedBy=multi-user.target", "WantedBy=" + target)
                (units / name).write_text(source)
                wants = units / (target + ".wants")
                wants.mkdir(exist_ok=True)
                (wants / name).symlink_to("../" + name)
            result = subprocess.run([shutil.which("systemd-analyze"), "--root=" + str(root),
                                     "--generators=no", "verify", "multi-user.target"],
                                    env={**os.environ, "SYSTEMD_LOG_LEVEL": "warning"},
                                    stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True, timeout=15)
            return result.returncode, result.stderr

    def test_late_private_sockets_have_no_boot_ordering_cycle(self):
        code, diagnostic = self.verify_graph()
        self.assertEqual(code, 0, diagnostic)
        self.assertNotIn("cycle", diagnostic.lower())

    def test_previous_socket_dependencies_form_a_real_cycle(self):
        for only_install_target in (False, True):
            with self.subTest(only_install_target=only_install_target):
                code, diagnostic = self.verify_graph(legacy=True, only_install_target=only_install_target)
                # systemd may return success after deleting a wanted job to
                # break the cycle. The diagnostic, not exit status, proves it.
                self.assertIn("cycle", diagnostic.lower())
                self.assertIn("deleted to break ordering cycle", diagnostic.lower())


if __name__ == "__main__":
    unittest.main()
