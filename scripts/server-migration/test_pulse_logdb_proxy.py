"""Endpoint guards and real Linux socket-activation stream regressions."""
import copy
import importlib.util
import json
import os
from pathlib import Path
import shutil
import socket
import subprocess
import sys
import threading
import time
import unittest
from unittest import mock


SPEC = importlib.util.spec_from_file_location("pulse_logdb_proxy", Path(__file__).with_name("pulse-logdb-proxy.py"))
proxy = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(proxy)


IMAGE = "sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b"


def inspected_fixture(address="172.18.0.3"):
    return {"Id": "a" * 64, "Name": "/newapi-mysql", "Image": IMAGE, "Running": True,
            "Mounts": [{"Type": "bind", "Source": "/opt/newapi/mysql_data", "Destination": "/var/lib/mysql", "RW": True}],
            "Networks": {"newapi_default": {"IPAddress": address}}}


def network_fixture():
    return {"Name": "newapi_default", "Driver": "bridge", "Internal": True,
            "IPAM": [{"Subnet": "172.18.0.0/16"}]}


class EndpointTests(unittest.TestCase):
    def test_dynamic_address_is_resolved_again_for_each_connection(self):
        for address in ("172.18.0.3", "172.18.0.27"):
            self.assertEqual(proxy.resolve_endpoint(inspected_fixture(address), network_fixture()), address)

    def test_wrong_container_identity_state_or_image_is_rejected(self):
        for field, value in (("Id", "invalid"), ("Id", ""), ("Name", "/newapi-mysql-other"),
                             ("Image", "mysql:8.0.46"), ("Image", "sha256:" + "b" * 64),
                             ("Running", False), ("Running", "true")):
            inspected = inspected_fixture()
            inspected[field] = value
            with self.subTest(field=field, value=value), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected, network_fixture())

    def test_missing_duplicate_or_wrong_data_mount_is_rejected(self):
        for field, value in (("Type", "volume"), ("Source", "/opt/newapi/old_mysql_data"),
                             ("Destination", "/var/lib/mysql-other"), ("RW", False)):
            inspected = inspected_fixture()
            inspected["Mounts"][0][field] = value
            with self.subTest(field=field, value=value), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected, network_fixture())
        for mounts in ([], inspected_fixture()["Mounts"] * 2):
            inspected = inspected_fixture()
            inspected["Mounts"] = mounts
            with self.subTest(mounts=mounts), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected, network_fixture())

    def test_network_must_be_the_single_owned_internal_bridge(self):
        for field, value in (("Name", "other"), ("Driver", "overlay"),
                             ("Internal", False), ("Internal", "true"),
                             ("IPAM", []), ("IPAM", [{"Subnet": "172.19.0.0/16"}])):
            network = network_fixture()
            network[field] = value
            with self.subTest(field=field, value=value), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected_fixture(), network)
        for networks in ({}, {"other": {"IPAddress": "172.18.0.3"}},
                         {"newapi_default": {"IPAddress": "172.18.0.3"}, "extra": {"IPAddress": "172.19.0.3"}}):
            inspected = inspected_fixture()
            inspected["Networks"] = networks
            with self.subTest(networks=networks), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected, network_fixture())

    def test_unsafe_or_outside_endpoint_is_rejected(self):
        for address in ("", "127.0.0.1", "8.8.8.8", "0.0.0.0", "224.0.0.1", "::1",
                        "172.19.0.3", "172.18.0.3,exec=unexpected", "localhost", None):
            with self.subTest(address=address), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected_fixture(address), network_fixture())

    def test_missing_inspection_fields_fail_closed(self):
        for field in inspected_fixture():
            inspected = inspected_fixture()
            del inspected[field]
            with self.subTest(field=field), self.assertRaises(proxy.ProxyError):
                proxy.resolve_endpoint(inspected, network_fixture())

    def test_main_executes_socat_without_a_shell_and_reinspects(self):
        values = [inspected_fixture(), network_fixture(), inspected_fixture("172.18.0.27"), network_fixture()]
        with mock.patch.object(proxy.sys, "argv", ["proxy"]), mock.patch.object(proxy, "read_json_command", side_effect=values) as read, mock.patch.object(proxy.os, "execv") as execute:
            proxy.main()
            proxy.main()
        self.assertEqual(read.call_count, 4)
        for address, call in zip(("172.18.0.3", "172.18.0.27"), execute.call_args_list):
            self.assertEqual(call.args, (proxy.SOCAT, [proxy.SOCAT, "-t", "60", "FD:0,shut-down",
                "TCP4:" + address + ":3306,connect-timeout=5,keepalive,nodelay"]))

    def test_invalid_endpoint_never_executes_transport(self):
        with mock.patch.object(proxy.sys, "argv", ["proxy"]), mock.patch.object(proxy, "read_json_command", side_effect=[inspected_fixture("8.8.8.8"), network_fixture()]), mock.patch.object(proxy.os, "execv") as execute:
            try:
                proxy.main()
            except proxy.ProxyError:
                pass
        execute.assert_not_called()


@unittest.skipUnless(sys.platform.startswith("linux") and shutil.which("socat"), "requires Linux and socat")
class SocketTransportTests(unittest.TestCase):
    def exercise_stream(self, backend_half_close=False):
        # Accept=yes provides a bidirectional accepted socket on fd0. Stdout is
        # /dev/null, matching the unit: protocol bytes must use FD:0,shut-down.
        # STDIO (even with shut-down) loses the backend EOF; FD:0 alone does too.
        # Each direction exceeds the relay/socket buffer by a wide margin and
        # includes every byte value, including NUL. No host port is published.
        request = bytes(range(256)) * 8192
        response = bytes(reversed(range(256))) * 8192
        listener = socket.socket()
        listener.bind(("127.0.0.1", 0))
        listener.listen(1)
        listener.settimeout(5)
        received, failures = [], []

        def backend():
            try:
                connection, _ = listener.accept()
                with connection:
                    connection.settimeout(5)
                    if backend_half_close:
                        connection.sendall(response)
                        connection.shutdown(socket.SHUT_WR)
                    chunks = []
                    while True:
                        data = connection.recv(65536)
                        if not data:
                            break
                        chunks.append(data)
                    received.append(b"".join(chunks))
                    if not backend_half_close:
                        # The response starts only AFTER the request EOF: this
                        # catches relays that close both directions together.
                        time.sleep(0.05)
                        connection.sendall(response)
                        connection.shutdown(socket.SHUT_WR)
            except Exception as error:
                failures.append(type(error).__name__)

        worker = threading.Thread(target=backend, daemon=True)
        worker.start()
        client, activated = socket.socketpair()
        client.settimeout(5)
        # Docker discovery is mocked only inside this child; actual endpoint
        # guards are covered above. Execute the wrapper's real socat options.
        child = r'''
import importlib.util, os, sys
spec = importlib.util.spec_from_file_location("proxy", sys.argv[1])
proxy = importlib.util.module_from_spec(spec)
spec.loader.exec_module(proxy)
proxy.read_json_command = lambda _args: {}
proxy.resolve_endpoint = lambda _inspected, _network: "127.0.0.1"
real_exec = os.execv
port = sys.argv[2]
sys.argv = [sys.argv[1]]
def isolated_exec(path, args):
    args[-1] = args[-1].replace("TCP4:127.0.0.1:3306,", "TCP4:127.0.0.1:" + port + ",", 1)
    real_exec(path, args)
proxy.os.execv = isolated_exec
proxy.main()
'''
        process = subprocess.Popen(
            [sys.executable, "-B", "-c", child, str(Path(proxy.__file__)), str(listener.getsockname()[1])],
            stdin=activated, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        activated.close()
        try:
            if not backend_half_close:
                client.sendall(request)
                client.shutdown(socket.SHUT_WR)
            chunks = []
            while True:
                data = client.recv(65536)
                if not data:
                    break
                chunks.append(data)
            if backend_half_close:
                client.sendall(request)
                client.shutdown(socket.SHUT_WR)
            self.assertEqual(b"".join(chunks), response)
            worker.join(5)
            self.assertFalse(worker.is_alive(), "backend did not observe the half-close")
            self.assertEqual(failures, [])
            self.assertEqual(received, [request])
            self.assertEqual(process.wait(timeout=5), 0)
            self.assertEqual(process.stderr.read(), b"")
        finally:
            client.close()
            listener.close()
            if process.poll() is None:
                process.kill()
                process.wait(timeout=5)
            process.stderr.close()

    def test_binary_request_half_close_keeps_response_direction_open(self):
        self.exercise_stream()

    def test_binary_backend_half_close_keeps_request_direction_open(self):
        self.exercise_stream(backend_half_close=True)


if __name__ == "__main__":
    unittest.main()
