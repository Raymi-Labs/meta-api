#!/usr/bin/env python3
"""Read-only, bounded-memory CRC/transaction/DDL audit of a fixed filepos range.

The source UUID must be bound separately using a live MySQL read. This tool
never queries MySQL, changes its files, skips events, or emits SQL/row payloads.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import stat
import sys

sys.dont_write_bytecode = True
STORAGE = Path(__file__).resolve().parents[1] / "server-storage"
sys.path.insert(0, str(STORAGE))
import pitr_binlog
spec = importlib.util.spec_from_file_location("source_audit_restore", STORAGE / "mysql-pitr-restore.py")
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)


class AuditError(Exception):
    pass


def require(condition, message):
    if not condition:
        raise AuditError(message)


def point(value):
    filename, separator, position = value.rpartition(":")
    require(separator and position.isascii() and position.isdigit(), "coordinate must be filename:position")
    return restore.coordinate({"file": filename, "position": int(position)})


def filenames(start, stop):
    a, first = start["file"].rsplit(".", 1)
    b, last = stop["file"].rsplit(".", 1)
    require(a == b and len(first) == len(last) and restore.position_key(start) <= restore.position_key(stop),
            "range is reversed or uses incompatible filenames")
    require(int(last) - int(first) <= 10000, "range contains too many files")
    return [a + "." + str(number).zfill(len(first)) for number in range(int(first), int(last) + 1)]


def read_exact(stream, count):
    data = stream.read(count)
    require(len(data) == count, "binlog ended before the fixed snapshot boundary")
    return data


def audit(directory, start, stop):
    start, stop = restore.coordinate(start), restore.coordinate(stop)
    directory = Path(directory)
    require(directory.is_dir(), "binlog directory is absent")
    state, files, server_ids = None, [], set()
    anchor_seen = start["position"] == 4
    total_events = selected_events = selected_transactions = query_events = 0
    for filename in filenames(start, stop):
        path = directory / filename
        fd = os.open(path, os.O_RDONLY | os.O_NOFOLLOW)
        with os.fdopen(fd, "rb", buffering=1024 * 1024) as stream:
            before = os.fstat(stream.fileno())
            require(stat.S_ISREG(before.st_mode), "binlog is not a regular file")
            limit = stop["position"] if filename == stop["file"] else before.st_size
            require(4 < limit <= before.st_size, "snapshot boundary exceeds available bytes")
            prefix = read_exact(stream, 4)
            require(prefix == pitr_binlog.MAGIC, "binlog magic differs")
            offset, prefix_hash, selected_hash, file_events = 4, hashlib.sha256(), hashlib.sha256(), 0
            range_start = start["position"] if filename == start["file"] else 0
            final_safe = False
            while offset < limit:
                require(limit - offset >= pitr_binlog.HEADER.size, "snapshot boundary splits an event header")
                header = read_exact(stream, pitr_binlog.HEADER.size)
                _, kind, server_id, length, end, _ = pitr_binlog.HEADER.unpack(header)
                require(pitr_binlog.HEADER.size + 4 <= length <= pitr_binlog.MAX_EVENT,
                        "event length exceeds the bounded parser contract")
                require(offset + length <= limit, "snapshot boundary splits an event payload")
                body = read_exact(stream, length - pitr_binlog.HEADER.size)
                chunk_start = 0 if offset == 4 else offset
                chunk = prefix + header + body if offset == 4 else header + body
                parsed = pitr_binlog.parse_chunk(chunk, filename, chunk_start, state)
                restore.validate_query_scope(parsed["data"], filename, chunk_start, start, stop, "new-api")
                state = parsed["state"]
                prefix_hash.update(parsed["data"])
                # Prefix bytes are parsed to reconstruct trustworthy framing;
                # only the requested suffix contributes to the migration range.
                if end > range_start:
                    relative = max(0, range_start - chunk_start)
                    selected_hash.update(parsed["data"][relative:])
                server_ids.add(server_id)
                total_events += 1
                file_events += 1
                if restore.position_key({"file": filename, "position": end}) > restore.position_key(start):
                    selected_events += 1
                    selected_transactions += len(parsed["boundaries"])
                    query_events += int(kind == 2)
                if filename == start["file"] and end == start["position"]:
                    require(end in parsed["safe_boundaries"], "backup anchor is inside a transaction")
                    anchor_seen = True
                offset = end
                final_safe = end in parsed["safe_boundaries"]
            require(offset == limit and final_safe, "snapshot boundary is not a safe transaction boundary")
            after = os.fstat(stream.fileno())
            current = path.lstat()
            require((current.st_dev, current.st_ino) == (before.st_dev, before.st_ino)
                    and after.st_size >= limit, "binlog was replaced or truncated while reading")
            if filename != stop["file"]:
                require(after.st_size == before.st_size, "closed binlog size changed while reading")
            files.append({"file": filename, "range_start": range_start, "range_stop": limit,
                          "parsed_events": file_events, "normalized_prefix_sha256": prefix_hash.hexdigest(),
                          "selected_range_sha256": selected_hash.hexdigest()})
    require(anchor_seen, "backup anchor is not an event boundary")
    require(state and not state["in_transaction"] and not state["pending_gtid"],
            "audit stopped inside an unfinished transaction")
    result = {"format_version": 1, "status": "scope-verified", "database": "new-api",
              "start": start, "stop": stop, "files": files, "server_ids": sorted(server_ids),
              "parsed_events": total_events, "selected_events": selected_events,
              "selected_transactions": selected_transactions, "selected_query_events": query_events,
              "maximum_event_bytes": pitr_binlog.MAX_EVENT,
              "normalization": "fde-clear-in-use-v1", "source_uuid_verified": False}
    result["evidence_sha256"] = hashlib.sha256(pitr_binlog.canonical_json(result)).hexdigest()
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--directory", required=True)
    parser.add_argument("--start", required=True, type=point)
    parser.add_argument("--stop", required=True, type=point)
    args = parser.parse_args()
    print(json.dumps(audit(args.directory, args.start, args.stop), sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (AuditError, restore.RestoreError, OSError, ValueError, KeyError, TypeError):
        print("Binlog audit failed: no verified scope receipt was produced; source files were not modified.", file=sys.stderr)
        sys.exit(1)
