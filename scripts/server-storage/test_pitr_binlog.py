import base64
import copy
import hashlib
import struct
import unittest
import zlib

from pitr_binlog import (HEADER, MAGIC, MAX_EVENT, canonical_json, normalize,
                        parse_chunk, receipt_digest, segment_name, validate_receipt)


def event(kind, payload, position, flags=0):
    size = HEADER.size + len(payload) + 4
    value = HEADER.pack(1700000000, kind, 123, size, position + size, flags) + payload
    return value + struct.pack("<I", zlib.crc32(value))


def fde():
    payload = struct.pack("<H", 4) + b"8.0.46".ljust(50, b"\0") + b"\0" * 4 + bytes([19]) + b"\0" * 41 + b"\x01"
    return event(15, payload, 4)


def query(sql, position):
    db = b"new-api"
    payload = struct.pack("<IIBHH", 7, 0, len(db), 0, 0) + db + b"\0" + sql
    return event(2, payload, position)


def fixture(rows=1):
    data = MAGIC + fde()
    data += event(35, b"\0" * 8, len(data))
    for i in range(rows):
        data += event(34, b"\0" * 42, len(data))
        data += query(b"BEGIN", len(data))
        data += event(19, b"\0" * 30, len(data))
        data += event(30, b"\0" * 30, len(data))
        data += event(16, struct.pack("<Q", i + 1), len(data))
    return data


class ParserTests(unittest.TestCase):
    def test_real_mysql_8046_row_fixture(self):
        # Captured from isolated mysql:8.0.46 ARM64: CREATE DATABASE/TABLE,
        # INSERT quota=100, UPDATE quota=75. Contains only synthetic demo data.
        data = base64.b64decode(
            '/mJpbiVD2moPLR4BAHoAAAB+AAAAAQAEADguMC40NgAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
            'JUPaahMADQAIAAAAAAQABAAAAGIABBoIAAAACAgIAgAAAAoKCioqABI0AAooAAHse3/yJUPaaiMtHgEAHwAAAJ0AAACAAAAAAAAA'
            'AAAAbjNpIeJD2moiLR4BAE0AAADqAAAAAAABAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAgAAAAAAAAAAAQAAAAAAAAAwsFb2cF4G'
            'ua44AQCK6hmk4kPaagItHgEAbAAAAFYBAAAIAAkAAAAAAAAABAAALwAAAAAAAAEgAKBFAAAAAAYDc3RkBAgACAD/AAwBZGVtbwAR'
            'BgAAAAAAAAAS/wAUAGRlbW8AQ1JFQVRFIERBVEFCQVNFIGRlbW8+fNYt4kPaaiItHgEATQAAAKMBAAAAAAEAAAAAAAAAAAAAAAAA'
            'AAAAAAAAAAAAAAACAQAAAAAAAAACAAAAAAAAAIXSVvZwXgbrrjgBAFRnGoPiQ9pqAi0eAQCeAAAAQQIAAAAACQAAAAAAAAAAAAAv'
            'AAAAAAAAASAAoEUAAAAABgNzdGQECAAIAP8ADAFkZW1vABEHAAAAAAAAABL/ABMAAENSRUFURSBUQUJMRSBkZW1vLmFjY291bnRz'
            'KGlkIElOVCBQUklNQVJZIEtFWSwgcXVvdGEgQklHSU5UKSBFTkdJTkU9SW5ub0RCJutGc+JD2moiLR4BAE8AAACQAgAAAAAAAAAA'
            'AAAAAAAAAAAAAAAAAAAAAAAAAAAAAgIAAAAAAAAAAwAAAAAAAABc2Vb2cF4G/BwBrjgBAJBLCZHiQ9pqAi0eAQBHAAAA1wIAAAgA'
            'CQAAAAAAAAAAAAAdAAAAAAAAASAAoEUAAAAABgNzdGQECAAIAP8AEv8AAEJFR0lOrFbKn+JD2moTLR4BADcAAAAOAwAAAABVAAAA'
            'AAABAARkZW1vAAhhY2NvdW50cwACAwgAAgEBAHoWBEziQ9pqHi0eAQAwAAAAPgMAAAAAVQAAAAAAAQACAAL/AAEAAABkAAAAAAAA'
            'AJFwXXbiQ9pqEC0eAQAfAAAAXQMAAAAACAAAAAAAAAAWW7vw4kPaaiItHgEATwAAAKwDAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA'
            'AAAAAAACAwAAAAAAAAAEAAAAAAAAAMvcVvZwXgb8MwGuOAEADVACzuJD2moCLR4BAFAAAAD8AwAACAAJAAAAAAAAAAAAACYAAAAA'
            'AAABIACgRQAAAAAGA3N0ZAQIAAgA/wAJAQAAAAAAAAAS/wAAQkVHSU44hvGN4kPaahMtHgEANwAAADMEAAAAAFUAAAAAAAEABGRl'
            'bW8ACGFjY291bnRzAAIDCAACAQEAbJbJHOJD2mofLR4BAD4AAABxBAAAAABVAAAAAAABAAIAAv//AAEAAABkAAAAAAAAAAABAAAA'
            'SwAAAAAAAAAoAoVv4kPaahAtHgEAHwAAAJAEAAAAAAoAAAAAAAAAezLdug=='
)
        parsed = parse_chunk(data, "binlog.000004", 0)
        self.assertEqual(len(parsed["boundaries"]), 4)
        self.assertEqual(parsed["boundaries"][-1], len(data))
        cut = parsed["boundaries"][2]
        first = parse_chunk(data[:cut], "binlog.000004", 0)
        last = parse_chunk(data[cut:], "binlog.000004", cut, first["state"])
        self.assertEqual(last["state"], parsed["state"])

    def test_complete_row_transaction(self):
        data = fixture(2)
        parsed = parse_chunk(data, "binlog.000001", 0)
        self.assertEqual(len(parsed["boundaries"]), 2)
        self.assertEqual(parsed["last_complete_transaction_frontier"], {"file": "binlog.000001", "position": len(data)})

    def test_transaction_spans_chunks(self):
        data = fixture()
        cut = len(data) - 31
        first = parse_chunk(data[:cut], "binlog.000001", 0)
        self.assertTrue(first["state"]["in_transaction"])
        self.assertIsNone(first["last_complete_transaction_frontier"])
        second = parse_chunk(data[cut:], "binlog.000001", cut, first["state"])
        self.assertEqual(second["boundaries"], [len(data)])

    def test_fde_normalization_matches_closed_file_crc(self):
        closed = fixture()
        active = bytearray(closed)
        active[21] |= 1
        self.assertEqual(parse_chunk(active, "binlog.000001", 0)["data"], closed)
        self.assertEqual(normalize(active, 0), closed)

    def test_crc_corruption(self):
        data = bytearray(fixture())
        data[-8] ^= 1
        with self.assertRaisesRegex(ValueError, "CRC"):
            parse_chunk(data, "binlog.000001", 0)

    def test_truncated_payload(self):
        with self.assertRaisesRegex(ValueError, "truncated"):
            parse_chunk(fixture()[:-1], "binlog.000001", 0)

    def test_position_mismatch_even_with_valid_crc(self):
        data = bytearray(fixture())
        offset = len(data) - 31
        struct.pack_into("<I", data, offset + 13, len(data) + 1)
        struct.pack_into("<I", data, len(data) - 4, zlib.crc32(data[offset:-4]))
        with self.assertRaisesRegex(ValueError, "end_log_pos"):
            parse_chunk(data, "binlog.000001", 0)

    def test_unknown_xa_and_compression_fail_closed(self):
        for kind in (38, 40, 255):
            data = MAGIC + fde()
            data += event(kind, b"\0" * 20, len(data))
            with self.assertRaisesRegex(ValueError, "unsupported"):
                parse_chunk(data, "binlog.000001", 0)

    def test_row_without_transaction_rejected(self):
        data = MAGIC + fde()
        data += event(30, b"\0" * 10, len(data))
        with self.assertRaisesRegex(ValueError, "outside transaction"):
            parse_chunk(data, "binlog.000001", 0)

    def test_statement_dml_rejected(self):
        data = MAGIC + fde()
        data += event(34, b"\0" * 42, len(data))
        data += query(b"INSERT INTO t VALUES (1)", len(data))
        with self.assertRaisesRegex(ValueError, "unsupported statement"):
            parse_chunk(data, "binlog.000001", 0)

    def test_atomic_ddl_boundary(self):
        data = MAGIC + fde()
        data += event(34, b"\0" * 42, len(data))
        data += query(b"CREATE TABLE t (id int)", len(data))
        self.assertEqual(parse_chunk(data, "binlog.000001", 0)["boundaries"], [len(data)])

    def test_rotation_and_file_gap(self):
        data = fixture()
        data += event(4, struct.pack("<Q", 4) + b"binlog.000002", len(data))
        state = parse_chunk(data, "binlog.000001", 0)["state"]
        parse_chunk(MAGIC + fde(), "binlog.000002", 0, state)
        with self.assertRaisesRegex(ValueError, "gap"):
            parse_chunk(MAGIC + fde(), "binlog.000003", 0, state)

    def test_transaction_cannot_cross_file_boundary(self):
        state = parse_chunk(fixture()[:-31], "binlog.000001", 0)["state"]
        with self.assertRaisesRegex(ValueError, "crosses"):
            parse_chunk(MAGIC + fde(), "binlog.000002", 0, state)

    def test_huge_event_rejected_before_payload(self):
        data = MAGIC + fde()
        data += HEADER.pack(0, 30, 1, MAX_EVENT + 1, MAX_EVENT + 1 + len(data), 0)
        with self.assertRaisesRegex(ValueError, "bounds"):
            parse_chunk(data, "binlog.000001", 0)

    def test_path_rejected(self):
        with self.assertRaises(ValueError):
            parse_chunk(fixture(), "../binlog.000001", 0)


if __name__ == "__main__":
    unittest.main()
