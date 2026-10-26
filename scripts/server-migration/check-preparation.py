#!/usr/bin/env python3
"""Read-only check of the deliberately application-free preparation Compose.

This validates normalized Compose JSON, not filesystem ownership, live firewall
rules, image contents, database state or the contents of private configuration.
Neither the JSON nor Docker stderr is printed: either can contain credentials.
In particular, Compose 2.40.3 can expand env files despite --no-env-resolution.
"""
import argparse
import json
import re
import subprocess
import sys
from pathlib import PurePosixPath


class InvalidPreparation(ValueError):
    pass


MOUNTS = {
    "mysql": {
        "/var/lib/mysql": ("/opt/newapi/mysql_data", False),
        "/etc/mysql/conf.d/newapi.cnf": ("/opt/newapi/config/mysql.cnf", True),
        "/docker-entrypoint-initdb.d/99-preparation-readonly.sql":
            ("/opt/newapi/config/99-preparation-readonly.sql", True),
    },
    "redis": {
        "/data": ("/opt/newapi/redis_data", False),
        "/usr/local/etc/redis/redis.conf": ("/opt/newapi/config/redis.conf", True),
    },
}
SERVICE_KEYS = {
    "image", "container_name", "restart", "environment", "env_file", "volumes",
    "mem_limit", "stop_grace_period", "healthcheck", "logging", "networks",
    "command", "entrypoint",
}


def require(condition):
    if not condition:
        raise InvalidPreparation("preparation configuration rejected")


def unique_object(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result)
        result[key] = value
    return result


def validate(config):
    require(isinstance(config, dict) and set(config) <= {"name", "services", "networks"})
    require(config.get("name") == "newapi")
    networks = config.get("networks")
    require(isinstance(networks, dict) and set(networks) == {"default"})
    network = networks["default"]
    require(isinstance(network, dict) and set(network) <= {"name", "internal", "ipam", "driver"})
    require(network.get("name") == "newapi_default" and network.get("internal") is True)
    require(network.get("driver", "bridge") == "bridge" and network.get("ipam", {}) == {})
    services = config.get("services")
    require(isinstance(services, dict) and set(services) == {"mysql", "redis"})
    for name, service in services.items():
        require(isinstance(service, dict) and set(service) <= SERVICE_KEYS)
        require(service.get("container_name") == "newapi-" + name)
        require(isinstance(service.get("image"), str))
        require(re.fullmatch(name + r"@sha256:[0-9a-f]{64}", service["image"]) is not None)
        require(service.get("entrypoint") is None)
        require(service.get("restart") in ("no", "unless-stopped"))
        require(service.get("networks") == {"default": None}
                or service.get("networks") == {"default": {}})
        command = None if name == "mysql" else ["redis-server", "/usr/local/etc/redis/redis.conf"]
        require(service.get("command") == command)
        if name == "mysql":
            environment = service.get("environment")
            public = {"MYSQL_DATABASE": "new-api", "MYSQL_ROOT_HOST": "localhost", "TZ": "Asia/Shanghai"}
            require(isinstance(environment, dict))
            require(all(environment.get(key) == value for key, value in public.items()))
            env_files = service.get("env_file")
            if env_files:
                require(set(environment) == set(public))
                require(isinstance(env_files, list) and len(env_files) == 1)
                env_file = env_files[0]
                require(isinstance(env_file, dict) and set(env_file) <= {"path", "required"})
                path = env_file.get("path")
                require(isinstance(path, str) and PurePosixPath(path).is_absolute()
                        and PurePosixPath(path).name == ".env.db" and ".." not in PurePosixPath(path).parts)
                require(env_file.get("required", True) is True)
            else:
                # Older Compose resolves and removes env_file before rendering.
                # Accept only the expected complete credentials, never echo them.
                private = {"MYSQL_USER", "MYSQL_PASSWORD", "MYSQL_ROOT_PASSWORD"}
                require(set(environment) == set(public) | private)
                require(all(isinstance(environment[key], str) and 0 < len(environment[key]) <= 4096
                            and environment[key].strip() and not any(c in environment[key] for c in "\x00\r\n")
                            for key in private))
                require(re.fullmatch(r"[A-Za-z_][A-Za-z0-9_]{0,31}", environment["MYSQL_USER"]) is not None)
                require(environment["MYSQL_USER"].lower() != "root")
        else:
            require(not service.get("environment") and not service.get("env_file"))
        mounts = service.get("volumes")
        require(isinstance(mounts, list) and len(mounts) == len(MOUNTS[name]))
        seen = set()
        for mount in mounts:
            require(isinstance(mount, dict) and set(mount) <= {"type", "source", "target", "read_only", "bind"})
            target = mount.get("target")
            require(isinstance(target, str) and target in MOUNTS[name] and target not in seen)
            seen.add(target)
            source, read_only = MOUNTS[name][target]
            require(mount.get("type") == "bind" and mount.get("source") == source)
            require(mount.get("read_only", False) is read_only)
            bind = mount.get("bind")
            require(isinstance(bind, dict) and set(bind) <= {"create_host_path"})
            # compose-go v2.9.1 serializes false with omitempty, producing {}.
            # An absent bind object is different and must not pass this gate.
            require(bind.get("create_host_path", False) is False)
        require(seen == set(MOUNTS[name]))
        logging = service.get("logging")
        require(logging == {"driver": "local", "options": {"max-size": "20m", "max-file": "5"}})
    return {"configuration_valid": True, "application_absent": True,
            "ports_unpublished": True, "internal_network_only": True,
            "fixed_bind_contract": True, "runtime_verified": False}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    source = parser.add_mutually_exclusive_group(required=True)
    source.add_argument("--compose", metavar="FILE", help="read via docker compose config; never starts Docker resources")
    source.add_argument("--stdin", action="store_true", help="normalized Compose JSON; may contain secrets, so use a private pipe")
    args = parser.parse_args()
    try:
        if args.stdin:
            data = sys.stdin.buffer.read(2 * 1024 * 1024 + 1)
        else:
            result = subprocess.run(
                ["docker", "compose", "-f", args.compose, "config", "--no-env-resolution", "--format", "json"],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=30, check=False,
            )
            require(result.returncode == 0)
            data = result.stdout
        require(len(data) <= 2 * 1024 * 1024)
        config = json.loads(data, object_pairs_hook=unique_object)
        result = validate(config)
    except (InvalidPreparation, OSError, ValueError, TypeError, RecursionError, subprocess.TimeoutExpired):
        print(json.dumps({"configuration_valid": False, "runtime_verified": False}))
        return 1
    print(json.dumps(result, sort_keys=True))
    return 0


if __name__ == "__main__":
    sys.exit(main())
