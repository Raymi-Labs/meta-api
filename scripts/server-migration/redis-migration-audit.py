#!/usr/bin/env python3
"""Read-only Redis migration gate: hashed values and unchanged absolute expiry.

Requires stopped writers. No key names, values or credentials are emitted.
The exact RDB file must additionally be checked with SHA-256 and redis-check-rdb.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess
import sys


class AuditError(Exception):
    pass


LUA = r'''
local keys = redis.call('KEYS', '*')
if #keys > 10000 then return redis.error_reply('fixture-sized audit limit exceeded') end
local clock = redis.call('TIME')
local now = tonumber(clock[1])*1000 + math.floor(tonumber(clock[2])/1000)
local rows, size = {}, 0
for _, key in ipairs(keys) do
  local value = redis.call('DUMP', key)
  if value then
    size = size + #value
    if size > 64*1024*1024 then return redis.error_reply('audit byte limit exceeded') end
    local expiry = redis.call('PEXPIRETIME', key)
    if expiry ~= -2 then
      table.insert(rows, {redis.sha1hex(key), redis.call('TYPE', key).ok,
                         redis.sha1hex(value), expiry})
    end
  end
end
return {now, rows}
'''


class DockerRedis:
    def __init__(self, container, container_id, image_id, auth_config=None, auth_env=None, auth_command=False):
        if not re.fullmatch(r"[a-zA-Z0-9][a-zA-Z0-9_.-]*", container):
            raise AuditError("invalid container name")
        if not re.fullmatch(r"[a-f0-9]{64}", container_id) or not re.fullmatch(r"sha256:[a-f0-9]{64}", image_id):
            raise AuditError("full immutable container and image IDs are required")
        if sum(map(bool, (auth_config, auth_env, auth_command))) != 1:
            raise AuditError("choose exactly one private authentication source")
        if auth_config and not re.fullmatch(r"/[a-zA-Z0-9/_.-]+", auth_config):
            raise AuditError("invalid container authentication config path")
        if auth_env and not re.fullmatch(r"[A-Z][A-Z0-9_]*", auth_env):
            raise AuditError("invalid authentication environment name")
        self.container, self.container_id, self.image_id = container, container_id, image_id
        self.auth_config, self.auth_env, self.auth_command = auth_config, auth_env, auth_command

    def run(self, arguments, env=None):
        result = subprocess.run(arguments, text=True, capture_output=True, timeout=45, env=env)
        if result.returncode:
            raise AuditError("Redis audit command failed; private output suppressed")
        return result.stdout

    def guard(self):
        actual = self.run(["docker", "inspect", "-f", "{{.Id}} {{.Image}} {{.State.Status}}", self.container]).strip()
        if actual != f"{self.container_id} {self.image_id} running":
            raise AuditError("Redis container identity or state differs")

    def cli(self, *arguments):
        # Credential material is read inside the container and exists only in
        # the client environment. It is never passed in docker/redis-cli argv.
        # redis-cli forces raw text for INFO even with --json; EVAL preserves
        # JSON framing, without introducing a write operation.
        if arguments and arguments[0] == "INFO":
            arguments = ("EVAL", "return redis.call('INFO', ARGV[1])", "0", arguments[1])
        if self.auth_command:
            command = json.loads(self.run(["docker", "inspect", "-f", "{{json .Config.Cmd}}", self.container_id]))
            if not isinstance(command, list) or command.count("--requirepass") != 1:
                raise AuditError("expected one private requirepass argument")
            index = command.index("--requirepass") + 1
            if index >= len(command) or not isinstance(command[index], str) or not command[index]:
                raise AuditError("invalid private authentication argument")
            # docker reads the value from this subprocess environment; the
            # command line contains only the name REDISCLI_AUTH, never its value.
            output = self.run(["docker", "exec", "-e", "REDISCLI_AUTH", self.container_id,
                               "redis-cli", "--no-auth-warning", "--json", *map(str, arguments)],
                              env={**os.environ, "REDISCLI_AUTH": command[index]})
        elif self.auth_config:
            script = 'set -eu; REDISCLI_AUTH="$(sed -n "s/^requirepass[[:space:]]\\{1,\\}//p" "$1")"; shift; test -n "$REDISCLI_AUTH"; export REDISCLI_AUTH; exec redis-cli --no-auth-warning --json "$@"'
            source = self.auth_config
        else:
            script = 'set -eu; REDISCLI_AUTH="$(printenv "$1")"; shift; test -n "$REDISCLI_AUTH"; export REDISCLI_AUTH; exec redis-cli --no-auth-warning --json "$@"'
            source = self.auth_env
        if not self.auth_command:
            output = self.run(["docker", "exec", self.container_id, "sh", "-c", script, "audit", source, *map(str, arguments)])
        try:
            return json.loads(output)
        except (ValueError, TypeError):
            raise AuditError("invalid Redis audit response") from None

    def snapshot(self):
        self.guard()
        info = self.cli("INFO", "server")
        if not isinstance(info, str) or "redis_version:7.4.9\r\n" not in info:
            raise AuditError("Redis 7.4.9 is required")
        databases = self.cli("CONFIG", "GET", "databases")
        if not isinstance(databases, dict):
            databases = dict(zip(databases[::2], databases[1::2]))
        count = int(databases["databases"])
        if not 1 <= count <= 64:
            raise AuditError("unexpected database count")
        result = {"format_version": 1, "redis_version": "7.4.9", "databases": {}}
        for database in range(count):
            now, rows = self.cli("-n", database, "EVAL", LUA, "0")
            entries = {}
            for key_hash, kind, value_hash, expires_at in rows:
                if not re.fullmatch("[a-f0-9]{40}", key_hash) or not re.fullmatch("[a-f0-9]{40}", value_hash) or key_hash in entries:
                    raise AuditError("invalid or duplicate key fingerprint")
                entries[key_hash] = {"type": kind, "dump_sha1": value_hash, "expires_at_ms": expires_at}
            result["databases"][str(database)] = {"observed_at_ms": now, "entries": entries}
        self.guard()
        return result


def compare(expected, actual):
    if expected.get("format_version") != 1 or expected.get("redis_version") != "7.4.9" or expected.get("databases", {}).keys() != actual["databases"].keys():
        raise AuditError("Redis snapshot format or database scope differs")
    expired, retained = 0, 0
    for database, before in expected["databases"].items():
        after = actual["databases"][database]
        if after["observed_at_ms"] < before["observed_at_ms"] - 1000:
            raise AuditError("target clock is behind the snapshot clock")
        for key_hash, entry in before["entries"].items():
            current = after["entries"].get(key_hash)
            if current is None:
                if entry["expires_at_ms"] > 0 and entry["expires_at_ms"] <= after["observed_at_ms"]:
                    expired += 1
                    continue
                raise AuditError("a live key is missing after migration")
            if current != entry:
                raise AuditError("key type, serialized value or absolute expiration differs")
            retained += 1
        if after["entries"].keys() - before["entries"].keys():
            raise AuditError("unexpected keys after migration")
    return {"verified": True, "retained_keys": retained, "naturally_expired_keys": expired,
            "absolute_expiration_unchanged": True}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("snapshot", "verify"))
    parser.add_argument("--container", required=True)
    parser.add_argument("--container-id", required=True)
    parser.add_argument("--image-id", required=True)
    auth = parser.add_mutually_exclusive_group(required=True)
    auth.add_argument("--auth-config")
    auth.add_argument("--auth-env")
    auth.add_argument("--auth-command", action="store_true", help="read requirepass privately from container Config.Cmd")
    parser.add_argument("--manifest", required=True, type=Path)
    args = parser.parse_args()
    try:
        client = DockerRedis(args.container, args.container_id, args.image_id, args.auth_config, args.auth_env, args.auth_command)
        current = client.snapshot()
        if args.action == "snapshot":
            descriptor = os.open(args.manifest, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(descriptor, "w") as output:
                json.dump(current, output, sort_keys=True)
                output.write("\n")
                output.flush()
                os.fsync(output.fileno())
            print(json.dumps({"snapshot_saved": True, "keys": sum(len(db["entries"]) for db in current["databases"].values())}))
        else:
            print(json.dumps(compare(json.loads(args.manifest.read_text()), current)))
    except (AuditError, OSError, ValueError, KeyError, TypeError, subprocess.SubprocessError):
        print("Redis migration audit failed; no Redis data or configuration was modified; inspect private evidence.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
