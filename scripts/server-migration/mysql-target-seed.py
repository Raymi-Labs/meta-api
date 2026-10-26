#!/usr/bin/env python3
"""Explicit permanent-target migration gate; never connects to the source.

Run on the already prepared target only. No promotion, purge, reset, container
restart, credential printing, or automatic retry of a partial seed is provided.
"""
import argparse
import contextlib
import fcntl
import gzip
import hashlib
import importlib.util
import ipaddress
import json
import os
from pathlib import Path
import re
import select
import secrets
import shlex
import shutil
import signal
import stat
import subprocess
import sys
import time

STORAGE = Path(__file__).resolve().parents[1] / "server-storage"
sys.path.insert(0, str(STORAGE))
import pitr_anchor
import pitr_audit
spec = importlib.util.spec_from_file_location("migration_restore", STORAGE / "mysql-pitr-restore.py")
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)
DB = "new-api"
PACKET = 256 * 1024 * 1024
UUID = re.compile(r"[0-9a-f]{8}(?:-[0-9a-f]{4}){3}-[0-9a-f]{12}")


class MigrationError(Exception):
    """Diagnostics contain fixed text only, never SQL or subprocess output."""


def require(condition, message):
    if not condition:
        raise MigrationError(message)


def private_file(path):
    path = Path(path)
    info = path.lstat()
    require(stat.S_ISREG(info.st_mode) and not info.st_mode & 0o077,
            "input must be a private regular file")
    require(info.st_uid == os.geteuid(), "input file has an unexpected owner")
    return path


def read_json(path):
    path = private_file(path)
    require(path.stat().st_size <= 32 * 1024 * 1024, "JSON evidence exceeds limit")
    value = json.loads(path.read_bytes())
    require(isinstance(value, dict), "JSON evidence must be an object")
    return value


def write_json(path, value):
    path = Path(path)
    require(path.parent.is_dir() and not path.parent.stat().st_mode & 0o077,
            "receipt directory must be private")
    with path.open("x") as stream:
        os.chmod(path, 0o600)
        json.dump(value, stream, sort_keys=True)
        stream.write("\n")
        stream.flush()
        os.fsync(stream.fileno())


def validate_bundle(directory, source_uuid, expected_manifest):
    directory = Path(directory)
    manifest_path = private_file(directory / "manifest.json")
    raw = manifest_path.read_bytes()
    require(hashlib.sha256(raw).hexdigest() == expected_manifest, "manifest fingerprint differs from approved backup")
    manifest = read_json(manifest_path)
    marker = read_json(directory / "SUCCESS.json")
    source, dump = manifest.get("source", {}), manifest.get("dump", {})
    require(marker.get("status") == "success" and marker.get("format_version") == 1
            and marker.get("manifest_sha256") == expected_manifest
            and marker.get("run_id") == manifest.get("run_id")
            and manifest.get("scope") == "database-only", "backup completion marker does not match manifest")
    require(source.get("server_uuid") == source_uuid
            and source.get("mysql_version", "").split("-")[0] == "8.0.46"
            and source.get("database") == DB and source.get("database_scope") == [DB],
            "backup source identity or scope differs")
    anchor = source.get("binlog_anchor", {})
    require(anchor.get("server_uuid") == source_uuid, "backup anchor UUID differs")
    point = restore.coordinate(anchor)
    require(dump.get("name") == "mysql.sql.gz" and type(dump.get("size")) is int
            and dump["size"] > 0 and re.fullmatch(r"[0-9a-f]{64}", dump.get("sha256", "")),
            "backup dump fingerprint is invalid")
    path = private_file(directory / "mysql.sql.gz")
    with path.open("rb") as stream:
        actual = restore.backup.gzip_digest(stream, dump["size"])
    require(actual == {key: dump[key] for key in ("size", "sha256")}, "backup gzip fingerprint differs")
    header = pitr_anchor.Header()
    with gzip.open(path, "rb") as stream:
        prefix = stream.read(65537)
        header.feed(prefix)
    require(header.finish() == point, "dump anchor differs from manifest")
    require(b"SQL_LOG_BIN" not in prefix.upper() and b"GTID_PURGED" not in prefix.upper(),
            "dump unexpectedly changes session binlogging or GTID state")
    return {"path": path, "anchor": point, "manifest_sha256": expected_manifest,
            "dump_sha256": dump["sha256"], "source_uuid": source_uuid}


class Target:
    def __init__(self, target_uuid, source_uuid, container="newapi-mysql"):
        require(UUID.fullmatch(target_uuid) and UUID.fullmatch(source_uuid)
                and target_uuid != source_uuid, "source and target UUIDs must differ")
        require(container == "newapi-mysql", "unsupported target container")
        self.target_uuid, self.source_uuid, self.container = target_uuid, source_uuid, container
        self.container_id = None

    def run(self, args, **kwargs):
        result = subprocess.run(args, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, **kwargs)
        require(result.returncode == 0, "target Docker or SQL command failed")
        return result.stdout

    def assert_container(self):
        data = json.loads(self.run(["docker", "inspect", self.container]))[0]
        if self.container_id is not None:
            require(data["Id"] == self.container_id, "target container changed during migration")
        mounts = [m for m in data.get("Mounts", []) if m.get("Destination") == "/var/lib/mysql"]
        require(len(mounts) == 1 and mounts[0].get("Type") == "bind"
                and mounts[0].get("Source") == "/opt/newapi/mysql_data"
                and mounts[0].get("RW") is True and data.get("State", {}).get("Running") is True,
                "target data mount or running state differs")
        ports = data.get("HostConfig", {}).get("PortBindings")
        require(not ports or ports == {"3306/tcp": [{"HostIp": "127.0.0.1", "HostPort": "3306"}]},
                "target MySQL port binding is not the approved loopback-only endpoint")
        self.container_id = data["Id"]

    def command(self, *, quick=False):
        self.assert_container()
        args = ["mysql", "--protocol=SOCKET", "--user=root", "--max-allowed-packet=256M",
                "--batch", "--raw", "--skip-column-names", "--default-character-set=utf8mb4",
                "--binary-mode", "--local-infile=0", "--skip-reconnect", "--unbuffered"]
        if quick:
            args.append("--quick")
        return ["docker", "exec", "-i", self.container_id, "sh", "-c",
                'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec ' + shlex.join(args)]

    def sql(self, sql):
        return self.run(self.command(), input=("SET SESSION time_zone='+00:00';\n" + sql + "\n").encode())

    def query(self, sql):
        data = self.sql(sql).decode().rstrip("\n")
        if sql.startswith("SHOW CREATE"):
            name, ddl = data.split("\t", 1)
            if sql.startswith("SHOW CREATE VIEW"):
                ddl = ddl.rsplit("\t", 2)[0]
            return [[name, ddl]]
        return [json.loads(line) for line in data.splitlines()]

    def lines(self, sql):
        child = subprocess.Popen(self.command(quick=True), stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                 stderr=subprocess.DEVNULL)
        try:
            child.stdin.write(("SET SESSION time_zone='+00:00';\n" + sql + "\n").encode())
            child.stdin.close()
            yield from child.stdout
            require(child.wait() == 0, "target row fingerprint query failed")
        finally:
            if child.poll() is None:
                child.terminate()
                child.wait()
            child.stdout.close()

    def guard(self, *, empty=False):
        row = self.query("SELECT JSON_ARRAY(@@version,@@server_uuid,@@server_id,@@gtid_mode,"
                         "@@binlog_format,@@read_only,@@super_read_only,@@event_scheduler,"
                         "@@master_info_repository,@@relay_log_info_repository,@@skip_replica_start)")[0]
        require(row[0].split("-")[0] == "8.0.46" and row[1] == self.target_uuid
                and row[2] not in (0, 1) and row[3:7] == ["OFF", "ROW", 1, 1]
                and row[7:] == ["OFF", "TABLE", "TABLE", 1], "target identity or isolation settings differ")
        schemas = self.query("SELECT JSON_ARRAY(SCHEMA_NAME) FROM information_schema.SCHEMATA WHERE "
                             "SCHEMA_NAME NOT IN ('mysql','information_schema','performance_schema','sys','new-api')")
        require(not schemas, "target contains unexpected schemas")
        if empty:
            count = self.query("SELECT JSON_ARRAY(COUNT(*)) FROM information_schema.TABLES WHERE TABLE_SCHEMA='new-api'")[0][0]
            channels = self.query("SELECT JSON_ARRAY(COUNT(*)) FROM performance_schema.replication_connection_configuration")[0][0]
            require(count == 0 and channels == 0, "target is not empty or already has replication metadata")

    def lock_writes(self):
        self.sql("SET GLOBAL super_read_only=ON; SET PERSIST_ONLY read_only=ON; SET PERSIST_ONLY super_read_only=ON;")

    def load(self, path):
        child = subprocess.Popen(self.command(), stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                 stderr=subprocess.DEVNULL)
        connection_id, completed, token = None, False, secrets.token_hex(24)
        try:
            child.stdin.write(("SET SESSION sql_log_bin=0; SET @newapi_migration_seed='" + token
                               + "'; SELECT CONNECTION_ID();\n").encode())
            child.stdin.flush()
            require(select.select([child.stdout], [], [], 10)[0], "import connection handshake timed out")
            identity = child.stdout.readline(64).strip()
            require(re.fullmatch(rb"[1-9][0-9]*", identity), "import connection identity is invalid")
            connection_id = int(identity)
            with gzip.open(path, "rb") as stream:
                shutil.copyfileobj(stream, child.stdin, 1024 * 1024)
            child.stdin.close()
            require(child.wait() == 0, "permanent target import failed; partial data retained and locked")
            completed = True
        finally:
            try:
                if not completed and connection_id is not None:
                    # Cancel the server session, not just the docker CLI. Bind
                    # its numeric ID to our random session token before KILL.
                    owned = self.query("SELECT JSON_ARRAY(COUNT(*)) FROM performance_schema.user_variables_by_thread v "
                        "JOIN performance_schema.threads t ON t.THREAD_ID=v.THREAD_ID WHERE "
                        f"t.PROCESSLIST_ID={connection_id} AND v.VARIABLE_NAME='newapi_migration_seed' "
                        f"AND CONVERT(v.VARIABLE_VALUE USING utf8mb4)='{token}'")[0][0]
                    if owned:
                        self.sql(f"KILL CONNECTION {connection_id};")
            finally:
                if child.poll() is None:
                    child.terminate()
                    try:
                        child.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        child.kill()
                        child.wait()
                for stream in (child.stdin, child.stdout):
                    if stream and not stream.closed:
                        stream.close()


def seed(target, bundle, reference, receipt):
    require(not Path(receipt).exists(), "seed receipt already exists")
    require(reference.get("status") == "success"
            and reference.get("backup_b_manifest_sha256") == bundle["manifest_sha256"]
            and reference.get("source_uuid") == target.source_uuid
            and reference.get("database") == DB
            and reference.get("differences") == [], "reference recovery is not bound to this backup")
    expected = reference.get("reference_audit")
    require(isinstance(expected, dict) and expected.get("database") == DB and expected.get("tables"),
            "verified backup reference audit is absent")
    target.guard(empty=True)
    try:
        target.sql(f"SET PERSIST max_allowed_packet={PACKET}; SET GLOBAL super_read_only=OFF;")
        target.load(bundle["path"])
    finally:
        target.lock_writes()
    target.guard()
    actual = pitr_audit.audit_database(target.query, target.lines, DB)
    differences = pitr_audit.compare_audits(expected, actual)
    require(not differences, "seed differs from verified backup reference audit")
    result = {"format_version": 1, "status": "seed-verified", "source_uuid": target.source_uuid,
              "target_uuid": target.target_uuid, "database": DB, "anchor": bundle["anchor"],
              "manifest_sha256": bundle["manifest_sha256"], "dump_sha256": bundle["dump_sha256"],
              "table_count": len(actual["tables"]),
              "audit_sha256": hashlib.sha256(pitr_audit.canonical(actual)).hexdigest()}
    write_json(receipt, result)
    return result


def sql_string(value):
    require(isinstance(value, str) and value and "\x00" not in value and "\n" not in value and "\r" not in value,
            "invalid replication connection value")
    return "'" + value.replace("'", "''") + "'"


def replication_sql(receipt, connection):
    point = restore.coordinate(receipt["anchor"])
    require(set(connection) == {"host", "port", "user", "password"}, "unexpected replication connection fields")
    address = ipaddress.ip_address(connection["host"])
    require(address.version == 4 and (address.is_loopback or address in ipaddress.ip_network("172.16.0.0/12")
            or address in ipaddress.ip_network("10.0.0.0/8") or address in ipaddress.ip_network("192.168.0.0/16")),
            "replication endpoint must be the private encrypted tunnel endpoint")
    require(type(connection["port"]) is int and 1024 <= connection["port"] <= 65535,
            "replication tunnel port is invalid")
    require(re.fullmatch(r"newapi_migrate_[a-zA-Z0-9_]+", connection["user"]), "replication user is not migration-scoped")
    require(isinstance(connection["password"], str) and 1 <= len(connection["password"].encode()) <= 32,
            "MySQL replication password must be at most 32 bytes")
    # NO_BACKSLASH_ESCAPES makes quote doubling safe even for arbitrary passwords.
    # DO_DB is essential: table wildcards alone do not filter account statements.
    return ("SET SESSION sql_mode='NO_BACKSLASH_ESCAPES';\n"
            "SET PERSIST replica_parallel_workers=0; SET PERSIST sync_relay_log=1;\n"
            "CHANGE REPLICATION FILTER REPLICATE_DO_DB=(`new-api`);\n"
            "CHANGE REPLICATION SOURCE TO "
            f"SOURCE_HOST={sql_string(connection['host'])},SOURCE_PORT={connection['port']},"
            f"SOURCE_USER={sql_string(connection['user'])},SOURCE_PASSWORD={sql_string(connection['password'])},"
            f"SOURCE_LOG_FILE={sql_string(point['file'])},SOURCE_LOG_POS={point['position']},"
            "SOURCE_AUTO_POSITION=0,SOURCE_SSL=0,GET_SOURCE_PUBLIC_KEY=1,SOURCE_CONNECT_RETRY=5;\n"
            "START REPLICA IO_THREAD;\n")


def start_replica(target, receipt, connection, source_ddl_frozen):
    require(source_ddl_frozen, "source DDL must be audited and frozen before replication")
    require(receipt.get("status") == "seed-verified" and receipt.get("format_version") == 1
            and receipt.get("source_uuid") == target.source_uuid
            and receipt.get("target_uuid") == target.target_uuid and receipt.get("database") == DB,
            "seed receipt does not identify this target")
    target.guard()
    require(target.query("SELECT JSON_ARRAY(COUNT(*)) FROM performance_schema.replication_connection_configuration")[0][0] == 0,
            "replication metadata already exists; refusing to rewind")
    try:
        target.sql(replication_sql(receipt, connection))
        for _ in range(30):
            rows = target.query("SELECT JSON_ARRAY(SOURCE_UUID,SERVICE_STATE,LAST_ERROR_NUMBER) "
                                "FROM performance_schema.replication_connection_status")
            require(len(rows) == 1 and rows[0][2] == 0, "replication source connection failed")
            if rows[0][0]:
                require(rows[0][0] == target.source_uuid, "tunnel reached an unexpected source UUID")
                if rows[0][1] == "ON":
                    target.sql("START REPLICA SQL_THREAD;")
                    return
            time.sleep(1)
        raise MigrationError("source identity was not verified before apply")
    except BaseException:
        target.sql("STOP REPLICA;")
        raise


@contextlib.contextmanager
def migration_lock(path="/opt/newapi/.migration-seed.lock"):
    fd = os.open(path, os.O_CREAT | os.O_RDWR | os.O_NOFOLLOW, 0o600)
    try:
        fcntl.flock(fd, fcntl.LOCK_EX | fcntl.LOCK_NB)
        yield
    finally:
        os.close(fd)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("action", choices=("seed", "start-replica"))
    parser.add_argument("--target-uuid", required=True)
    parser.add_argument("--source-uuid", required=True)
    parser.add_argument("--receipt", required=True)
    parser.add_argument("--bundle")
    parser.add_argument("--manifest-sha256")
    parser.add_argument("--reference-result")
    parser.add_argument("--connection-file")
    parser.add_argument("--source-ddl-audited-and-frozen", action="store_true")
    args = parser.parse_args()
    os.umask(0o077)
    require(os.geteuid() == 0, "run locally as root on the prepared target")
    def interrupted(_signum, _frame):
        raise MigrationError("migration interrupted; target seed cleanup will lock writes")
    signal.signal(signal.SIGTERM, interrupted)
    signal.signal(signal.SIGINT, interrupted)
    target = Target(args.target_uuid, args.source_uuid)
    with migration_lock():
        if args.action == "seed":
            require(args.bundle and args.manifest_sha256 and args.reference_result, "seed evidence arguments are required")
            bundle = validate_bundle(args.bundle, args.source_uuid, args.manifest_sha256)
            result = seed(target, bundle, read_json(args.reference_result), args.receipt)
            print(json.dumps(result, sort_keys=True))
        else:
            require(args.connection_file, "private replication connection file is required")
            start_replica(target, read_json(args.receipt), read_json(args.connection_file), args.source_ddl_audited_and_frozen)
            print('{"replication_start_requested":true,"caught_up":false,"promoted":false}')


if __name__ == "__main__":
    try:
        main()
    except (MigrationError, restore.RestoreError, restore.backup.BackupError, pitr_anchor.AnchorError,
            OSError, ValueError, KeyError, TypeError, EOFError):
        print("Migration gate failed; no promotion was performed. Inspect private evidence and target read-only state.", file=sys.stderr)
        sys.exit(1)
