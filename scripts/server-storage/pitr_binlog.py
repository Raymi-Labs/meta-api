"""Strict MySQL 8.0 ROW binlog framing and the immutable PITR archive protocol.

Only the FDE BINLOG_IN_USE flag is normalized. No event is filtered or rewritten.
This intentionally fails closed for XA, compressed transactions and statement DML.
"""
import copy
import hashlib
import json
import re
import struct
import zlib

MAGIC = b"\xfebin"
HEADER = struct.Struct("<IBIIIH")
MAX_EVENT = 64 * 1024 * 1024
FILE_RE = re.compile(r"[A-Za-z0-9_-]+\.[0-9]{6,}")
SHA_RE = re.compile(r"[0-9a-f]{64}")


def canonical_json(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":")) + "\n").encode()


def receipt_digest(value):
    return hashlib.sha256(canonical_json(value)).hexdigest()


def valid_file(value):
    if not isinstance(value, str) or not FILE_RE.fullmatch(value):
        raise ValueError("invalid binlog filename")
    return value


def follows(previous, current):
    valid_file(previous)
    valid_file(current)
    a, x = previous.rsplit(".", 1)
    b, y = current.rsplit(".", 1)
    return a == b and int(y) == int(x) + 1


def normalize(data, start):
    if start != 0:
        return bytes(data)
    if len(data) < 23 or data[:4] != MAGIC or data[8] != 15:
        raise ValueError("binlog must start with magic and format description")
    if not data[21] & 1:
        return bytes(data)
    value = bytearray(data)
    value[21] &= ~1
    return bytes(value)


def query_text(payload):
    if len(payload) < 13:
        raise ValueError("truncated QUERY event")
    db_length = payload[8]
    status_length = struct.unpack_from("<H", payload, 11)[0]
    offset = 13 + status_length + db_length
    if offset >= len(payload) or payload[offset] != 0:
        raise ValueError("invalid QUERY event database header")
    return bytes(payload[offset + 1:]).strip().rstrip(b";").upper()


def parse_chunk(data, file, start, state=None):
    """Parse whole events, preserving transaction state between bounded chunks."""
    valid_file(file)
    result = copy.deepcopy(state) if state else {}
    if result and file != result["file"]:
        if start != 0 or not follows(result["file"], file):
            raise ValueError("binlog file sequence has a gap")
        if result.get("in_transaction") or result.get("pending_gtid"):
            raise ValueError("transaction crosses a binlog file boundary")
        rotate = result.get("rotate_to")
        if rotate is not None and rotate != file:
            raise ValueError("ROTATE target does not match next file")
        result.pop("fde_sha256", None)
        result.pop("rotate_to", None)
    elif result and start != result["next_position"]:
        raise ValueError("binlog byte range has a gap or overlap")
    elif not result and start != 0:
        raise ValueError("first binlog chunk must include file header")
    value = normalize(data, start)
    view = memoryview(value)
    offset = 4 if start == 0 else 0
    result.update(file=file, in_transaction=result.get("in_transaction", False),
                  pending_gtid=result.get("pending_gtid", False))
    boundaries, safe = [], []
    while offset < len(value):
        if len(value) - offset < HEADER.size:
            raise ValueError("truncated event header")
        timestamp, kind, server_id, length, end, flags = HEADER.unpack_from(value, offset)
        if length < HEADER.size + 4 or length > MAX_EVENT:
            raise ValueError("event length outside supported bounds")
        if offset + length > len(value):
            raise ValueError("truncated event payload")
        position = start + offset
        if end != position + length:
            raise ValueError("event end_log_pos does not match its byte position")
        event = view[offset:offset + length]
        if zlib.crc32(event[:-4]) != struct.unpack_from("<I", event, length - 4)[0]:
            raise ValueError("event CRC32 mismatch")
        payload = event[HEADER.size:-4]
        committed = False
        if kind == 15:
            if position != 4 or len(payload) < 58 or payload[-1] != 1:
                raise ValueError("only MySQL v4 CRC32 format descriptions are supported")
            if struct.unpack_from("<H", payload)[0] != 4 or payload[56] != 19:
                raise ValueError("unsupported binlog format version or event header size")
            result["fde_sha256"] = hashlib.sha256(event).hexdigest()
        elif "fde_sha256" not in result:
            raise ValueError("event before format description")
        elif kind in (33, 34):  # GTID / ANONYMOUS_GTID
            if result["in_transaction"] or result["pending_gtid"]:
                raise ValueError("new GTID before prior transaction completed")
            result["pending_gtid"] = True
        elif kind == 2:
            query = query_text(payload)
            if query == b"BEGIN":
                if result["in_transaction"] or not result["pending_gtid"]:
                    raise ValueError("unexpected BEGIN")
                result["in_transaction"] = True
            elif query in (b"COMMIT", b"ROLLBACK"):
                if not result["in_transaction"]:
                    raise ValueError("transaction terminator outside transaction")
                result["in_transaction"] = result["pending_gtid"] = False
                committed = True
            elif query.startswith(b"XA "):
                raise ValueError("XA transactions are not supported")
            elif query.startswith((b"SAVEPOINT ", b"ROLLBACK TO ", b"RELEASE SAVEPOINT ")):
                if not result["in_transaction"]:
                    raise ValueError("savepoint outside transaction")
            elif (not result["in_transaction"] and result["pending_gtid"]
                  and re.match(rb"^(CREATE|ALTER|DROP|RENAME|TRUNCATE|GRANT|REVOKE|ANALYZE|OPTIMIZE|REPAIR|FLUSH|RESET)\b", query)):
                result["pending_gtid"] = False
                committed = True
            else:
                raise ValueError("unsupported statement event; ROW transactions are required")
        elif kind == 16:  # XID
            if not result["in_transaction"] or len(payload) != 8:
                raise ValueError("invalid XID or XID outside transaction")
            result["in_transaction"] = result["pending_gtid"] = False
            committed = True
        elif kind in (19, 23, 24, 25, 30, 31, 32, 39):
            if not result["in_transaction"]:
                raise ValueError("row event outside transaction")
        elif kind == 4:
            if result["in_transaction"] or result["pending_gtid"] or len(payload) < 9:
                raise ValueError("invalid ROTATE event")
            if struct.unpack_from("<Q", payload)[0] != 4:
                raise ValueError("unsupported ROTATE position")
            target = bytes(payload[8:]).decode("ascii")
            if not follows(file, target):
                raise ValueError("ROTATE skips a binlog file")
            result["rotate_to"] = target
        elif kind in (3, 35):  # STOP / PREVIOUS_GTIDS
            if result["in_transaction"] or result["pending_gtid"]:
                raise ValueError("metadata event inside transaction")
        else:
            raise ValueError("unsupported binlog event type")
        offset += length
        if committed:
            boundaries.append(end)
            result["last_complete_transaction_frontier"] = {"file": file, "position": end}
        if not result["in_transaction"] and not result["pending_gtid"]:
            safe.append(end)
    if offset != len(value):
        raise ValueError("binlog chunk does not end at an event boundary")
    result["next_position"] = start + len(value)
    return {"data": value, "state": result, "boundaries": boundaries,
            "safe_boundaries": safe, "fde_sha256": result.get("fde_sha256"),
            "last_complete_transaction_frontier": result.get("last_complete_transaction_frontier")}


def segment_name(seq, file, start, end, sha256):
    valid_file(file)
    if not SHA_RE.fullmatch(sha256):
        raise ValueError("invalid segment SHA256")
    return f"segments/{seq:012d}-{file}-{start}-{end}-{sha256}.bin"


def validate_receipt(receipt, previous, root):
    required = ("seq", "source_uuid", "file", "start", "end", "size", "sha256", "object",
                "previous_receipt_sha256", "parser_state", "source_sample", "verified_at")
    if not isinstance(receipt, dict) or any(k not in receipt for k in required):
        raise ValueError("incomplete archive receipt")
    seq = previous["seq"] + 1 if previous else 1
    if receipt["seq"] != seq or receipt["source_uuid"] != root["uuid"]:
        raise ValueError("receipt sequence or source identity mismatch")
    expected = receipt_digest(previous) if previous else None
    if receipt["previous_receipt_sha256"] != expected:
        raise ValueError("receipt hash chain is broken")
    file, start, end = receipt["file"], receipt["start"], receipt["end"]
    if not all(type(n) is int for n in (start, end, receipt["size"])) or start < 0 or end <= start:
        raise ValueError("invalid archive byte range")
    if receipt["size"] != end - start:
        raise ValueError("segment size does not match byte range")
    if previous:
        if file == previous["file"]:
            if start != previous["end"]:
                raise ValueError("archive byte range gap or overlap")
        elif not follows(previous["file"], file) or start != 0:
            raise ValueError("archive file sequence gap")
    elif file != root["start_file"] or start != 0:
        raise ValueError("archive does not begin with root start file")
    if receipt["object"] != segment_name(seq, file, start, end, receipt["sha256"]):
        raise ValueError("segment name does not match its receipt")
    state = receipt["parser_state"]
    if state.get("file") != file or state.get("next_position") != end:
        raise ValueError("parser state does not match segment position")
