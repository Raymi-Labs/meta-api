#!/usr/bin/env python3
"""Read-only cutover audit: verified B + continuous binlogs + bounded logs suffix.

All non-logs tables are fully fingerprinted. Old logs rows are NOT rehashed;
their correctness depends on the verified B seed, audited continuous replication
and exact stopped execution position. Counts cannot detect old-row updates or
same-count replacement. Output is private; comparison prints only table/reason.
"""
import argparse
import hashlib
import importlib.util
import json
from pathlib import Path
import re
import sys

sys.dont_write_bytecode = True
HERE = Path(__file__).resolve().parent
SPEC = importlib.util.spec_from_file_location("final_audit_seed", HERE / "mysql-target-seed.py")
seed = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(seed)
audit = seed.pitr_audit
DB = "new-api"
LOGS = "`new-api`.`logs`"


def require(condition, message):
    seed.require(condition, message)


def read_bound(path, expected_sha256):
    require(re.fullmatch(r"[a-f0-9]{64}", expected_sha256 or ""), "invalid evidence fingerprint")
    path = seed.private_file(path)
    raw = path.read_bytes()
    require(len(raw) <= 32 * 1024 * 1024 and hashlib.sha256(raw).hexdigest() == expected_sha256,
            "private evidence fingerprint differs")
    value = json.loads(raw)
    require(isinstance(value, dict), "private evidence must be an object")
    return value


def bind_evidence(reference, receipt, chain, source_uuid, final):
    require(reference.get("status") == "success" and reference.get("verification") == "independent-snapshot-comparison"
            and reference.get("source_uuid") == source_uuid and reference.get("database") == DB
            and reference.get("differences") == [], "baseline independent recovery was not verified")
    baseline = reference.get("reference_audit", {})
    require(baseline.get("database") == DB and len(baseline.get("tables", {})) == 76
            and "logs" in baseline["tables"], "baseline must cover the 76-table business database")
    require(re.fullmatch(r"[a-f0-9]{64}", reference.get("backup_b_manifest_sha256", ""))
            and type(baseline["tables"]["logs"].get("count")) is int and baseline["tables"]["logs"]["count"] >= 0,
            "baseline manifest or logs count is invalid")
    require(audit.compare_audits(reference["incremental_audit"], baseline) == [], "baseline audits differ")
    baseline_point = seed.restore.coordinate(reference["chain"]["stop"])
    high = reference["replay_window"]["after"]["logs"]["max_id"]
    require(type(high) is int and high >= 0, "baseline logs high-water mark is invalid")
    require(receipt.get("status") == "seed-verified" and receipt.get("format_version") == 1
            and receipt.get("source_uuid") == source_uuid and receipt.get("database") == DB
            and receipt.get("manifest_sha256") == reference.get("backup_b_manifest_sha256")
            and receipt.get("anchor") == baseline_point
            and receipt.get("table_count") == 76
            and re.fullmatch(r"[a-f0-9]{64}", receipt.get("audit_sha256", "")),
            "target seed is not bound to the fully verified B snapshot")
    require(seed.UUID.fullmatch(receipt.get("target_uuid", "")) and receipt["target_uuid"] != source_uuid,
            "seed target UUID is invalid")
    require(chain.get("status") == "scope-verified" and chain.get("format_version") == 1
            and chain.get("database") == DB and chain.get("start") == baseline_point
            and chain.get("stop") == final and chain.get("files"), "continuous binlog evidence does not span B to final")
    server_ids = set(chain.get("server_ids", [])) - {0}
    require(len(server_ids) == 1 and all(type(value) is int and value > 0 for value in server_ids),
            "continuous binlog server identity is ambiguous")
    return {"baseline_point": baseline_point, "logs_after_id": high,
            "baseline_logs_count": baseline["tables"]["logs"]["count"],
            "baseline_manifest_sha256": reference["backup_b_manifest_sha256"],
            # The seed gate already compared all logical fields. Raw DDL
            # allocator high-water marks may differ without a logical mismatch;
            # keep both evidence hashes rather than equating their raw JSON.
            "baseline_audit_sha256": hashlib.sha256(audit.canonical(baseline)).hexdigest(),
            "seed_audit_sha256": receipt["audit_sha256"],
            "source_server_id": next(iter(server_ids)), "target_uuid": receipt["target_uuid"]}


class FrozenDatabase(seed.Target):
    """Reuse only the established SQL transport, never Target's seed guard."""
    def __init__(self, role, expected_uuid, source_uuid, container, container_id, image_id, final, source_server_id):
        require(role in ("source", "target") and seed.UUID.fullmatch(expected_uuid)
                and seed.UUID.fullmatch(source_uuid), "invalid final audit identity")
        require((expected_uuid == source_uuid) == (role == "source"), "audit role and UUID differ")
        require(container == "newapi-mysql" and re.fullmatch("[a-f0-9]{64}", container_id)
                and re.fullmatch("sha256:[a-f0-9]{64}", image_id), "full expected MySQL container and image identities required")
        self.role, self.expected_uuid, self.source_uuid = role, expected_uuid, source_uuid
        self.container, self.container_id, self.image_id = container, container_id, image_id
        self.final, self.source_server_id = final, source_server_id

    def assert_container(self):
        # Do not read Config.Env or source credentials into the report.
        result = self.run(["docker", "inspect", "-f", "{{.Id}} {{.Image}} {{.State.Status}}", self.container]).decode().strip()
        require(result == f"{self.container_id} {self.image_id} running", "MySQL container identity or state differs")

    def replica_status(self):
        command = self.command()
        command[-1] += " --column-names"
        rows = self.run(command, input=b"SHOW REPLICA STATUS;\n").decode().splitlines()
        require(len(rows) == 2, "exactly one stopped replication channel is required")
        names, values = (row.split("\t") for row in rows)
        require(len(names) == len(values), "replication status fields are malformed")
        return dict(zip(names, values))

    def guard(self):
        row = self.query("SELECT JSON_ARRAY(@@version,@@server_uuid,@@server_id,@@gtid_mode,@@binlog_format,"
                         "@@read_only,@@super_read_only,@@innodb_flush_log_at_trx_commit,@@sync_binlog)")[0]
        require(row[0].split("-")[0] == "8.0.46" and row[1] == self.expected_uuid
                and row[3:] == ["OFF", "ROW", 1, 1, 1, 1], "MySQL identity, readonly or durability gate failed")
        if self.role == "source":
            require(row[2] == self.source_server_id, "source server ID differs from audited binlogs")
        scopes = self.query("SELECT JSON_ARRAY(SCHEMA_NAME) FROM information_schema.SCHEMATA WHERE "
                            "SCHEMA_NAME NOT IN ('mysql','information_schema','performance_schema','sys') ORDER BY SCHEMA_NAME")
        require(scopes == [[DB]], "unexpected business database scope")
        writers = self.query("SELECT JSON_ARRAY(COUNT(*)) FROM information_schema.innodb_trx WHERE trx_is_read_only=0 OR trx_rows_modified>0")[0][0]
        require(writers == 0, "active write-capable transaction exists")
        require(self.query("SELECT JSON_ARRAY(COUNT(*)) FROM performance_schema.replication_connection_status WHERE SERVICE_STATE='ON'")[0][0] == 0
                and self.query("SELECT JSON_ARRAY(COUNT(*)) FROM performance_schema.replication_applier_status WHERE SERVICE_STATE='ON'")[0][0] == 0,
                "replication must be stopped before final audit")
        if self.role == "source":
            fields = self.sql("SHOW MASTER STATUS;").decode().strip().split("\t")
            require(len(fields) >= 2 and {"file": fields[0], "position": int(fields[1])} == self.final,
                    "source final binlog position changed")
        else:
            status = self.replica_status()
            required = {"Source_UUID": self.source_uuid, "Source_Server_Id": str(self.source_server_id),
                        "Replica_IO_Running": "No", "Replica_SQL_Running": "No", "Last_IO_Errno": "0", "Last_SQL_Errno": "0",
                        "Relay_Source_Log_File": self.final["file"], "Exec_Source_Log_Pos": str(self.final["position"]),
                        "Auto_Position": "0", "Replicate_Do_DB": DB}
            require(all(status.get(key) == value for key, value in required.items()), "target executed position or replication identity differs")
            require(all(not status.get(key) for key in ("Replicate_Ignore_DB", "Replicate_Do_Table", "Replicate_Ignore_Table",
                        "Replicate_Wild_Do_Table", "Replicate_Wild_Ignore_Table", "Replicate_Rewrite_DB")), "unexpected replication filters")


class LogsSuffix:
    """Restrict only the exact pitr_audit-generated logs row/aggregate shapes."""
    def __init__(self, transport, high):
        require(type(high) is int and high >= 0, "invalid logs range boundary")
        self.transport, self.high = transport, high

    def rewrite(self, sql):
        marker = " FROM " + LOGS
        if marker not in sql or sql.startswith("SHOW CREATE"):
            return sql
        require(sql.count(marker) == 1 and " WHERE " not in sql and " JOIN " not in sql and ";" not in sql,
                "unexpected logs audit query shape")
        head, tail = sql.split(marker, 1)
        require((head.startswith("SELECT JSON_ARRAY(") and (not tail or tail.startswith(" GROUP BY ")))
                or (head.startswith("SELECT SHA2(") and " AS row_hash" in head and tail == " ORDER BY `id`"),
                "unrecognized logs audit query")
        return head + marker + f" WHERE `id`>{self.high}" + tail

    def query(self, sql):
        return self.transport.query(self.rewrite(sql))

    def lines(self, sql):
        return self.transport.lines(self.rewrite(sql))


def collect(transport, binding, final):
    transport.guard()
    columns = transport.query("SELECT JSON_ARRAY(COLUMN_NAME,DATA_TYPE) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA='new-api' AND TABLE_NAME='logs' AND COLUMN_NAME='id'")
    primary = transport.query("SELECT JSON_ARRAY(COLUMN_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='new-api' AND TABLE_NAME='logs' AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX")
    require(len(columns) == 1 and columns[0][0] == "id" and columns[0][1] in ("int", "bigint") and primary == [["id"]],
            "logs suffix requires an integer id primary key")
    scoped = LogsSuffix(transport, binding["logs_after_id"])
    result = audit.audit_database(scoped.query, scoped.lines, DB)
    require(len(result["tables"]) == 76 and all(item["kind"] == "BASE TABLE" for item in result["tables"].values()),
            "final business schema differs from the 76-table scope")
    high = binding["logs_after_id"]
    indexes = [row[0] for row in transport.query("SELECT JSON_ARRAY(INDEX_NAME) FROM information_schema.STATISTICS WHERE TABLE_SCHEMA='new-api' AND TABLE_NAME='logs' AND COLUMN_NAME='id' AND SEQ_IN_INDEX=1 AND INDEX_NAME<>'PRIMARY' AND IS_VISIBLE='YES' ORDER BY INDEX_NAME")]
    require(indexes, "logs count requires an existing visible id-leading secondary index")
    count_index = "idx_created_at_id" if "idx_created_at_id" in indexes else indexes[0]
    # Count a covering secondary index, not the clustered rows containing large
    # request bodies. MAX uses its own primary-key endpoint lookup.
    count_from = LOGS + " FORCE INDEX (" + audit.ident(count_index) + ")"
    total = transport.query(f"SELECT JSON_ARRAY(COUNT(*)) FROM {count_from}")[0][0]
    maximum = transport.query(f"SELECT JSON_ARRAY(MAX(`id`)) FROM {LOGS}")[0][0]
    prefix = transport.query(f"SELECT JSON_ARRAY(COUNT(*)) FROM {count_from} WHERE `id`<={high}")[0][0]
    suffix = result["tables"]["logs"]["count"]
    require(type(total) is int and type(prefix) is int and total == prefix + suffix,
            "logs prefix/suffix counts do not cover the table")
    transport.guard()
    return {"audit": result, "logs_coverage": {"suffix_predicate": f"id>{high}", "suffix_rows_hashed": suffix,
            "total_count": total, "max_id": maximum, "old_prefix_count": prefix,
            "count_covering_index": count_index,
            "baseline_prefix_count": binding["baseline_logs_count"], "old_prefix_rehashed": False},
            "scope": "verified-baseline-B-plus-continuous-binlog-plus-final-nonlogs-and-logs-suffix",
            "limitations": ["Old logs row values are not rehashed; UPDATE or same-count replacement below the boundary is not independently detected.",
                            "Correctness of the old prefix relies on the fully verified B seed, audited continuous binlogs, readonly target and exact executed final coordinate."],
            "final_coordinate": final}


def compare(left, right):
    differences = []
    for key in ("format_version", "source_uuid", "final_coordinate", "binding", "evidence", "scope"):
        if left.get(key) != right.get(key):
            differences.append("evidence:" + key)
    if left.get("role") != "source" or right.get("role") != "target":
        differences.append("evidence:role")
    differences += audit.compare_audits(left["audit"], right["audit"])
    if left.get("logs_coverage") != right.get("logs_coverage"):
        differences.append("logs:coverage-counts-or-range")
    return differences


def main(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="action", required=True)
    snapshot = commands.add_parser("snapshot")
    snapshot.add_argument("--role", choices=("source", "target"), required=True)
    for name in ("expected-uuid", "source-uuid", "container-id", "image-id", "reference", "reference-sha256",
                 "seed-receipt", "seed-receipt-sha256", "binlog-audit", "binlog-audit-sha256", "final-file", "output"):
        snapshot.add_argument("--" + name, required=True)
    snapshot.add_argument("--final-position", type=int, required=True)
    snapshot.add_argument("--container", default="newapi-mysql")
    diff = commands.add_parser("compare")
    diff.add_argument("--source", required=True)
    diff.add_argument("--target", required=True)
    args = parser.parse_args(argv)
    try:
        if args.action == "compare":
            differences = compare(seed.read_json(args.source), seed.read_json(args.target))
            print(json.dumps({"matched": not differences, "differences": differences,
                              "scope": "baseline-B-and-continuous-binlogs; old logs prefix not rehashed"}))
            return bool(differences)
        reference = read_bound(args.reference, args.reference_sha256)
        receipt = read_bound(args.seed_receipt, args.seed_receipt_sha256)
        chain = read_bound(args.binlog_audit, args.binlog_audit_sha256)
        final = seed.restore.coordinate({"file": args.final_file, "position": args.final_position})
        binding = bind_evidence(reference, receipt, chain, args.source_uuid, final)
        require(args.role != "target" or args.expected_uuid == binding["target_uuid"], "target UUID differs from seed receipt")
        transport = FrozenDatabase(args.role, args.expected_uuid, args.source_uuid, args.container,
                                   args.container_id, args.image_id, final, binding["source_server_id"])
        report = collect(transport, binding, final)
        report.update(format_version=1, role=args.role, source_uuid=args.source_uuid, server_uuid=args.expected_uuid,
                      container_id=args.container_id, image_id=args.image_id,
                      runtime_guards_before_and_after={"readonly": True, "super_readonly": True,
                      "no_write_transactions": True, "replication_stopped": True, "final_coordinate_matched": True},
                      binding=binding, evidence={"reference_sha256": args.reference_sha256,
                      "seed_receipt_sha256": args.seed_receipt_sha256, "binlog_audit_sha256": args.binlog_audit_sha256})
        seed.write_json(args.output, report)
        print(json.dumps({"private_report_saved": True, "role": args.role, "tables": 76,
                          "old_logs_prefix_rehashed": False}))
        return 0
    except (seed.MigrationError, seed.restore.RestoreError, OSError, ValueError, KeyError, TypeError, IndexError):
        print("Final migration audit failed; database unchanged; private SQL/results were not printed.", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
