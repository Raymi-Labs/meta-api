#!/usr/bin/env python3
"""One accepted private TCP connection to the verified current MySQL container.

No database credentials, Docker mutations or published ports are needed. The
systemd template supplies the accepted duplex socket as stdin; socat preserves
binary data, backpressure and TCP half-close semantics.
"""
import ipaddress
import json
import os
import re
import subprocess
import sys

DOCKER = "/usr/bin/docker"
SOCAT = "/usr/bin/socat"
MYSQL_CONTAINER = "newapi-mysql"
MYSQL_IMAGE = "sha256:7dcddc01f13bab2f15cde676d44d01f61fc9f99fe7785e86196dfc07d358ae2b"
NETWORK = "newapi_default"
DATA_DIR = "/opt/newapi/mysql_data"


class ProxyError(Exception):
    pass


def require(condition):
    if not condition:
        raise ProxyError("private MySQL endpoint identity failed")


def _resolve_endpoint(inspected, network):
    require(isinstance(inspected, dict) and isinstance(network, dict))
    require(re.fullmatch(r"[a-f0-9]{64}", inspected.get("Id", ""))
            and inspected.get("Name") == "/" + MYSQL_CONTAINER
            and inspected.get("Image") == MYSQL_IMAGE and inspected.get("Running") is True)
    mounts = [m for m in inspected.get("Mounts", []) if m.get("Destination") == "/var/lib/mysql"]
    require(len(mounts) == 1 and mounts[0].get("Type") == "bind"
            and mounts[0].get("Source") == DATA_DIR and mounts[0].get("RW") is True)
    networks = inspected.get("Networks", {})
    require(set(networks) == {NETWORK} and network.get("Name") == NETWORK
            and network.get("Driver") == "bridge" and network.get("Internal") is True)
    address = ipaddress.ip_address(networks[NETWORK].get("IPAddress", ""))
    require(address.version == 4 and address.is_private and not address.is_loopback
            and not address.is_unspecified and not address.is_multicast and not address.is_link_local)
    subnets = [ipaddress.ip_network(item["Subnet"], strict=True)
               for item in network.get("IPAM", []) if item.get("Subnet")]
    require(any(subnet.version == 4 and address in subnet
                and address not in (subnet.network_address, subnet.broadcast_address)
                for subnet in subnets))
    return str(address)


def resolve_endpoint(inspected, network):
    try:
        return _resolve_endpoint(inspected, network)
    except (ValueError, TypeError, KeyError, AttributeError):
        raise ProxyError("private MySQL endpoint identity failed") from None


def read_json_command(arguments):
    result = subprocess.run([DOCKER, "--host", "unix:///var/run/docker.sock", *arguments],
                            stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, timeout=5,
                            env={"PATH": "/usr/sbin:/usr/bin:/sbin:/bin", "HOME": "/nonexistent"})
    require(result.returncode == 0 and len(result.stdout) <= 1024 * 1024)
    return json.loads(result.stdout)


def main():
    try:
        require(len(sys.argv) == 1)
        # Inspect only identity, mounts and network fields, never Config.Env/Cmd.
        inspected = read_json_command(["inspect", "--format",
            '{"Id":{{json .Id}},"Name":{{json .Name}},"Image":{{json .Image}},'
            '"Running":{{json .State.Running}},"Mounts":{{json .Mounts}},'
            '"Networks":{{json .NetworkSettings.Networks}}}', MYSQL_CONTAINER])
        network = read_json_command(["network", "inspect", "--format",
            '{"Name":{{json .Name}},"Driver":{{json .Driver}},"Internal":{{json .Internal}},'
            '"IPAM":{{json .IPAM.Config}}}', NETWORK])
        address = resolve_endpoint(inspected, network)
        os.execv(SOCAT, [SOCAT, "-t", "60", "FD:0,shut-down",
                        "TCP4:" + address + ":3306,connect-timeout=5,keepalive,nodelay"])
    except (ProxyError, OSError, ValueError, TypeError, KeyError, AttributeError, subprocess.SubprocessError):
        print("Private MySQL proxy failed; endpoint details and credentials suppressed.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
