"""Deterministic, secret-free audit of an offline MySQL recovery.

The caller supplies SQL transports to its own isolated, stopped-writer database.
This module never connects to a server and never returns raw row values.
"""
import hashlib
import json
import re


FINANCIAL_COLUMNS = {
    "users": ("quota", "transferable_quota", "pulse_paid_quota", "used_quota", "request_count", "aff_quota", "aff_history"),
    "top_ups": ("amount", "money", "paid_money", "presentment_money"),
    "subscription_orders": ("money", "purchase_quantity"),
    "user_subscriptions": ("amount_total", "amount_used"),
    "subscription_pre_consume_records": ("pre_consumed",),
    "pulse_wallet_reservations": ("quota", "paid_quota"),
    "pulse_funding_ledgers": ("quota_delta", "paid_delta", "paid_after"),
    "tokens": ("remain_quota", "used_quota", "package_used_quota", "package_limit_quota"),
    "redemptions": ("quota", "transferable_quota"),
    "tasks": ("quota",),
    "logs": ("quota", "prompt_tokens", "completion_tokens"),
    "channels": ("used_quota",),
    "channel_quota_usages": ("used_quota", "used_count"),
    "invite_commission_ledgers": ("base_quota", "commission_quota", "settled_quota"),
    "aff_withdrawals": ("quota", "amount_cents"),
    "payment_risk_cases": ("expected_amount", "expected_money", "received_money", "applied_quota_delta"),
}
GROUP_COLUMNS = {
    "top_ups": ("status", "payment_provider", "payment_method", "paid_currency", "presentment_currency"),
    "subscription_orders": ("status", "payment_provider", "payment_method"),
    "logs": ("type",), "tokens": ("billing_mode", "package_enabled"),
    "pulse_funding_ledgers": ("kind",),
    "benefit_change_records": ("action", "benefit_type", "source_type", "target_type"),
}


def ident(value):
    if not isinstance(value, str) or not value or "\x00" in value:
        raise ValueError("invalid SQL identifier")
    return "`" + value.replace("`", "``") + "`"


def literal(value):
    return "CONVERT(0x" + value.encode("utf8").hex() + " USING utf8mb4)"


def canonical(value):
    return (json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True) + "\n").encode()


def _ddl_spans(ddl):
    """Yield code/opaque spans without rewriting quoted values or comments."""
    start = offset = 0
    while offset < len(ddl):
        char = ddl[offset]
        line_comment = char == "#" or (ddl.startswith("--", offset) and
            (offset + 2 == len(ddl) or ddl[offset + 2].isspace()))
        if char in "'\"`" or line_comment or ddl.startswith("/*", offset):
            yield True, ddl[start:offset]
            opaque_start = offset
            if char in "'\"`":
                quote = char
                offset += 1
                while offset < len(ddl):
                    if ddl[offset] == "\\" and quote != "`":
                        offset = min(offset + 2, len(ddl))
                    elif ddl[offset] == quote:
                        offset += 1
                        if offset < len(ddl) and ddl[offset] == quote:
                            offset += 1
                        else:
                            break
                    else:
                        offset += 1
            elif line_comment:
                while offset < len(ddl) and ddl[offset] not in "\r\n":
                    offset += 1
            else:
                end = ddl.find("*/", offset + 2)
                offset = len(ddl) if end == -1 else end + 2
            yield False, ddl[opaque_start:offset]
            start = offset
        else:
            offset += 1
    yield True, ddl[start:]


def normalize_create(ddl):
    # A same-charset explicit COLLATE already fixes the column character set.
    # SHOW CREATE can add redundant CHARACTER SET after dump/restore. Keep the
    # collation itself, all unrelated DDL and the separate raw DDL digest.
    redundant_charset = re.compile(
        r"\bCHARACTER\s+SET\s+([A-Z][A-Z0-9_]*)\s+"
        r"(?=COLLATE\s+([A-Z][A-Z0-9_]*)(?![A-Z0-9_]))", re.IGNORECASE)

    def normalize_code(code):
        code = redundant_charset.sub(
            lambda m: "" if m[2].lower().startswith(m[1].lower() + "_") else m[0], code)
        # The allocator high-water mark is not transactional: a rolled-back
        # insert can advance it during a consistent dump. Never alter literals.
        return re.sub(r"(?<=ENGINE=InnoDB) AUTO_INCREMENT=\d+", "", code)

    return "".join(normalize_code(span) if code else span for code, span in _ddl_spans(ddl))


def audit_database(query, lines, database):
    """query returns decoded JSON_ARRAY rows; lines streams scalar ASCII hashes."""
    db = literal(database)
    tables = query("SELECT JSON_ARRAY(TABLE_NAME,TABLE_TYPE) FROM information_schema.TABLES "
                   f"WHERE TABLE_SCHEMA={db} ORDER BY BINARY TABLE_NAME")
    result = {"format_version": 1, "database": database, "tables": {}, "objects": {}}
    settings = query("SELECT JSON_ARRAY(DEFAULT_CHARACTER_SET_NAME,DEFAULT_COLLATION_NAME) FROM information_schema.SCHEMATA "
                     f"WHERE SCHEMA_NAME={db}")
    if len(settings) != 1:
        raise ValueError("isolated recovery database does not exist")
    result["database_settings"] = settings[0]
    table_names = {row[0] for row in tables}
    for table, kind in tables:
        qualified = ident(database) + "." + ident(table)
        # SHOW CREATE output is transformed by the transport, never exposed.
        ddl = query("SHOW CREATE " + ("VIEW " if kind == "VIEW" else "TABLE ") + qualified)[0][1]
        item = {"kind": kind,
                "schema_sha256": hashlib.sha256(normalize_create(ddl).encode()).hexdigest(),
                "raw_schema_sha256": hashlib.sha256(ddl.encode()).hexdigest()}
        if kind == "BASE TABLE":
            columns = [row[0] for row in query("SELECT JSON_ARRAY(COLUMN_NAME) FROM information_schema.COLUMNS "
                       f"WHERE TABLE_SCHEMA={db} AND TABLE_NAME={literal(table)} ORDER BY ORDINAL_POSITION")]
            primary = [row[0] for row in query("SELECT JSON_ARRAY(COLUMN_NAME) FROM information_schema.STATISTICS "
                       f"WHERE TABLE_SCHEMA={db} AND TABLE_NAME={literal(table)} AND INDEX_NAME='PRIMARY' ORDER BY SEQ_IN_INDEX")]
            # HEX(BINARY) is unambiguous for NULL, NUL, delimiters, blobs and text.
            # The column order and types are bound by the schema digest.
            expression = "SHA2(CAST(JSON_ARRAY(" + ",".join("HEX(CAST(" + ident(c) + " AS BINARY))" for c in columns) + ") AS BINARY),256)"
            order = ",".join(ident(c) for c in primary) if primary else "row_hash"
            digest, count = hashlib.sha256(), 0
            for line in lines(f"SELECT {expression} AS row_hash FROM {qualified} ORDER BY {order}"):
                line = line.strip()
                if not re.fullmatch(b"[0-9a-f]{64}", line):
                    raise ValueError("invalid row digest from isolated database")
                digest.update(line + b"\n")
                count += 1
            expected = int(query(f"SELECT JSON_ARRAY(COUNT(*)) FROM {qualified}")[0][0])
            if count != expected:
                raise ValueError("table changed during offline audit")
            item.update(count=count, data_sha256=digest.hexdigest(), ordering="primary-key" if primary else "row-hash-multiset")
            sums = [c for c in FINANCIAL_COLUMNS.get(table, ()) if c in columns]
            groups = [c for c in GROUP_COLUMNS.get(table, ("status",)) if c in columns]
            if sums or table in GROUP_COLUMNS:
                # Grouping preserves currency/status semantics. Aggregate values
                # stay decimal strings; floating-point JSON never loses money.
                fields = [ident(c) for c in groups] + ["COUNT(*)"]
                fields += [f"CAST(COALESCE(SUM(CAST({ident(c)} AS DECIMAL(60,12))),0) AS CHAR)" for c in sums]
                suffix = (" GROUP BY " + ",".join(ident(c) for c in groups) + " ORDER BY " + ",".join(ident(c) for c in groups)) if groups else ""
                rows = query("SELECT JSON_ARRAY(" + ",".join(fields) + ") FROM " + qualified + suffix)
                item["financial"] = {"groups": groups, "sums": sums, "rows": rows}
                if table == "top_ups" and "subscription_orders" in table_names:
                    wallet_rows = query("SELECT JSON_ARRAY(" + ",".join(fields) + ") FROM " + qualified
                        + " WHERE NOT EXISTS (SELECT 1 FROM " + ident(database) + ".`subscription_orders` s WHERE s.trade_no="
                        + qualified + ".trade_no)" + suffix)
                    item["financial"]["wallet_only_rows"] = wallet_rows
        result["tables"][table] = item
    # Include non-table schema objects without disclosing routine bodies.
    for kind, table, name, fields in (
        ("routines", "ROUTINES", "ROUTINE_NAME", "ROUTINE_NAME,ROUTINE_TYPE,DTD_IDENTIFIER,ROUTINE_DEFINITION,IS_DETERMINISTIC,SQL_DATA_ACCESS,SECURITY_TYPE,SQL_MODE,DEFINER,CHARACTER_SET_CLIENT,COLLATION_CONNECTION,DATABASE_COLLATION"),
        ("triggers", "TRIGGERS", "TRIGGER_NAME", "TRIGGER_NAME,EVENT_MANIPULATION,EVENT_OBJECT_TABLE,ACTION_ORDER,ACTION_STATEMENT,ACTION_ORIENTATION,ACTION_TIMING,SQL_MODE,DEFINER,CHARACTER_SET_CLIENT,COLLATION_CONNECTION,DATABASE_COLLATION"),
        ("events", "EVENTS", "EVENT_NAME", "EVENT_NAME,DEFINER,TIME_ZONE,EVENT_DEFINITION,EVENT_TYPE,EXECUTE_AT,INTERVAL_VALUE,INTERVAL_FIELD,SQL_MODE,STARTS,ENDS,STATUS,ON_COMPLETION,EVENT_COMMENT,CHARACTER_SET_CLIENT,COLLATION_CONNECTION,DATABASE_COLLATION"),
    ):
        schema_field = {"routines": "ROUTINE_SCHEMA", "triggers": "TRIGGER_SCHEMA", "events": "EVENT_SCHEMA"}[kind]
        rows = query(f"SELECT JSON_ARRAY({fields}) FROM information_schema.{table} WHERE {schema_field}={db} ORDER BY BINARY {name}")
        result["objects"][kind] = {"count": len(rows), "sha256": hashlib.sha256(canonical(rows)).hexdigest()}
    parameters = query("SELECT JSON_ARRAY(SPECIFIC_NAME,ORDINAL_POSITION,PARAMETER_MODE,PARAMETER_NAME,DTD_IDENTIFIER,CHARACTER_SET_NAME,COLLATION_NAME,ROUTINE_TYPE) "
                       f"FROM information_schema.PARAMETERS WHERE SPECIFIC_SCHEMA={db} ORDER BY BINARY SPECIFIC_NAME,ORDINAL_POSITION")
    result["objects"]["routine_parameters"] = {"count": len(parameters), "sha256": hashlib.sha256(canonical(parameters)).hexdigest()}
    return result


def window_snapshot(query, database):
    """Indexed high water marks plus small state summaries, no full log scan."""
    tables = {r[0] for r in query("SELECT JSON_ARRAY(TABLE_NAME) FROM information_schema.TABLES WHERE TABLE_SCHEMA=" + literal(database))}
    result = {}
    for table in ("logs", "top_ups", "subscription_orders", "pulse_wallet_reservations", "subscription_pre_consume_records", "benefit_change_records"):
        if table not in tables:
            continue
        columns = {r[0]: r[1] for r in query("SELECT JSON_ARRAY(COLUMN_NAME,DATA_TYPE) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA=" + literal(database) + " AND TABLE_NAME=" + literal(table))}
        qualified = ident(database) + "." + ident(table)
        item = {}
        if columns.get("id") in ("tinyint", "smallint", "mediumint", "int", "bigint"):
            item["max_id"] = query(f"SELECT JSON_ARRAY(MAX(id)) FROM {qualified}")[0][0]
        if "status" in columns:
            item["status_counts"] = query(f"SELECT JSON_ARRAY(status,COUNT(*)) FROM {qualified} GROUP BY status ORDER BY status")
        result[table] = item
    return result


def window_changes(query, database, before, after):
    result = {"before": before, "after": after, "new_rows": {},
              "note": "New rows are bounded by the snapshot primary-key high water marks. Status summaries retain existing-row transitions; net differences are not transaction counts."}
    for table, item in before.items():
        value = item.get("max_id")
        if table not in after or "max_id" not in item:
            continue
        predicate = "1=1" if value is None else "id>" + literal(str(value))
        qualified = ident(database) + "." + ident(table)
        if table == "logs":
            rows = query(f"SELECT JSON_ARRAY(type,COUNT(*),COALESCE(SUM(quota),0)) FROM {qualified} WHERE {predicate} GROUP BY type ORDER BY type")
            result["new_rows"][table] = {"by_type": rows, "topup": 0, "consume": 0, "refund": 0}
            for kind, count, _ in rows:
                if kind in (1, 2, 6):
                    result["new_rows"][table][{1: "topup", 2: "consume", 6: "refund"}[kind]] = count
        else:
            result["new_rows"][table] = {"count": query(f"SELECT JSON_ARRAY(COUNT(*)) FROM {qualified} WHERE {predicate}")[0][0]}
    return result


def compare_audits(left, right):
    """Return only table names/reasons; never include row contents or credentials."""
    differences = []
    if left["database"] != right["database"]:
        differences.append("database")
    if left.get("database_settings") != right.get("database_settings"):
        differences.append("database-settings")
    for table in sorted(set(left["tables"]) | set(right["tables"])):
        a, b = left["tables"].get(table), right["tables"].get(table)
        if a is None or b is None:
            differences.append(table + ":missing")
            continue
        for field in ("kind", "schema_sha256", "count", "data_sha256", "financial"):
            if a.get(field) != b.get(field):
                differences.append(table + ":" + field)
    if left.get("objects") != right.get("objects"):
        differences.append("schema-objects")
    return differences
