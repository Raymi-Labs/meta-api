#!/usr/bin/env python3
"""Read-only Redis migration gate: hashed values and unchanged absolute expiry.

Requires stopped writers. No key names, values or credentials are emitted.
The exact RDB file must additionally be checked with SHA-256 and redis-check-rdb.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import stat
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
      local kind = redis.call('TYPE', key).ok
      local dump_hash, logical_hash = redis.sha1hex(value), false
      if kind == 'zset' then
        local members = redis.call('ZRANGE', key, 0, -1, 'WITHSCORES')
        local parts = {'zset-members-scores-v1;'}
        for _, item in ipairs(members) do
          size = size + #item + 32
          if size > 64*1024*1024 then return redis.error_reply('audit byte limit exceeded') end
          table.insert(parts, tostring(#item) .. ':' .. item)
        end
        logical_hash = redis.sha1hex(table.concat(parts))
      end
      table.insert(rows, {redis.sha1hex(key), kind, dump_hash, expiry, logical_hash})
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
        result = {"format_version": 2, "redis_version": "7.4.9", "databases": {}}
        for database in range(count):
            now, rows = self.cli("-n", database, "EVAL", LUA, "0")
            entries = {}
            for key_hash, kind, value_hash, expires_at, logical_hash in rows:
                if not re.fullmatch("[a-f0-9]{40}", key_hash) or not re.fullmatch("[a-f0-9]{40}", value_hash) or key_hash in entries:
                    raise AuditError("invalid or duplicate key fingerprint")
                if kind == "zset" and not re.fullmatch("[a-f0-9]{40}", str(logical_hash)):
                    raise AuditError("invalid sorted set fingerprint")
                entries[key_hash] = {"type": kind, "dump_sha1": value_hash, "expires_at_ms": expires_at,
                                     "value_sha1": logical_hash if kind == "zset" else value_hash,
                                     "fingerprint": "zset-members-scores-v1" if kind == "zset" else "redis-dump-v1"}
            result["databases"][str(database)] = {"observed_at_ms": now, "entries": entries}
        self.guard()
        return result


def compare(expected, actual):
    version = expected.get("format_version")
    if (version not in (1, 2) or actual.get("format_version") not in (1, 2)
            or (version == 2 and actual.get("format_version") != 2)
            or expected.get("redis_version") != "7.4.9" or actual.get("redis_version") != "7.4.9"
            or expected.get("databases", {}).keys() != actual["databases"].keys()):
        raise AuditError("Redis snapshot format or database scope differs")
    expired, retained, encoding_changes = 0, 0, 0
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
            if current.get("type") != entry.get("type") or current.get("expires_at_ms") != entry.get("expires_at_ms"):
                raise AuditError("key type or absolute expiration differs")
            if version == 1:
                # A v1 manifest has no semantic proof. Never infer one from a
                # target with a different DUMP, even if its type/TTL match.
                if current.get("dump_sha1") != entry.get("dump_sha1"):
                    raise AuditError("legacy serialized value differs")
            else:
                fingerprint = "zset-members-scores-v1" if entry["type"] == "zset" else "redis-dump-v1"
                for value in (entry, current):
                    if (value.get("fingerprint") != fingerprint
                            or not re.fullmatch("[a-f0-9]{40}", str(value.get("value_sha1", "")))
                            or not re.fullmatch("[a-f0-9]{40}", str(value.get("dump_sha1", "")))
                            or (entry["type"] != "zset" and value["value_sha1"] != value["dump_sha1"])):
                        raise AuditError("invalid version two value fingerprint")
                if current["value_sha1"] != entry["value_sha1"]:
                    raise AuditError("canonical Redis value differs")
                if current["dump_sha1"] != entry["dump_sha1"]:
                    encoding_changes += 1
            retained += 1
        if after["entries"].keys() - before["entries"].keys():
            raise AuditError("unexpected keys after migration")
    return {"verified": True, "retained_keys": retained, "naturally_expired_keys": expired,
            "absolute_expiration_unchanged": True, "zset_encoding_changes": encoding_changes}


def upgrade(expected, current, prior_sha256):
    """Resample frozen SOURCE only after proving continuity with its v1 DUMP."""
    if (expected.get("format_version") != 1 or current.get("format_version") != 2
            or not re.fullmatch("[a-f0-9]{64}", prior_sha256)):
        raise AuditError("upgrade requires a bound legacy snapshot and a new source sample")
    proof = compare(expected, current)
    return {**current, "continuity": {"prior_format_version": 1, "prior_manifest_sha256": prior_sha256,
                                     "legacy_dump_verified": True, **proof}}


def read_private(path):
    # Reject FIFOs/devices after fstat without blocking while opening them.
    descriptor = os.open(path, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK)
    with os.fdopen(descriptor, "rb") as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
            raise AuditError("manifest must be a private owned regular file")
        raw = source.read(16 * 1024 * 1024 + 1)
        if len(raw) > 16 * 1024 * 1024:
            raise AuditError("manifest exceeds safety limit")
    return raw


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("snapshot", "verify", "upgrade"))
    parser.add_argument("--container", required=True)
    parser.add_argument("--container-id", required=True)
    parser.add_argument("--image-id", required=True)
    auth = parser.add_mutually_exclusive_group(required=True)
    auth.add_argument("--auth-config")
    auth.add_argument("--auth-env")
    auth.add_argument("--auth-command", action="store_true", help="read requirepass privately from container Config.Cmd")
    parser.add_argument("--manifest", required=True, type=Path)
    parser.add_argument("--prior-manifest", type=Path, help="upgrade only: original private v1 source snapshot")
    parser.add_argument("--prior-sha256", help="upgrade only: previously recorded v1 manifest SHA-256")
    args = parser.parse_args()
    try:
        if (args.action == "upgrade") != bool(args.prior_manifest and args.prior_sha256):
            raise AuditError("upgrade requires both legacy manifest and its recorded SHA-256")
        if args.action != "upgrade" and (args.prior_manifest or args.prior_sha256):
            raise AuditError("legacy continuity options are only valid for upgrade")
        prior = None
        if args.action == "upgrade":
            raw = read_private(args.prior_manifest)
            if hashlib.sha256(raw).hexdigest() != args.prior_sha256:
                raise AuditError("legacy manifest fingerprint differs")
            prior = json.loads(raw)
        client = DockerRedis(args.container, args.container_id, args.image_id, args.auth_config, args.auth_env, args.auth_command)
        current = client.snapshot()
        if args.action == "upgrade":
            current = upgrade(prior, current, args.prior_sha256)
        if args.action in ("snapshot", "upgrade"):
            descriptor = os.open(args.manifest, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600)
            with os.fdopen(descriptor, "w") as output:
                json.dump(current, output, sort_keys=True)
                output.write("\n")
                output.flush()
                os.fsync(output.fileno())
            print(json.dumps({"snapshot_saved": True, "keys": sum(len(db["entries"]) for db in current["databases"].values())}))
        else:
            print(json.dumps(compare(json.loads(read_private(args.manifest)), current)))
    except (AuditError, OSError, ValueError, KeyError, TypeError, AttributeError, subprocess.SubprocessError):
        print("Redis migration audit failed; no Redis data or configuration was modified; inspect private evidence.", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
