#!/usr/bin/env python3
"""Real Linux MySQL 8.0.46 + mysqlbinlog + rclone crypt PITR integration.

Creates only labelled network-none Docker containers and synthetic data. Requires
the separately built newapi-pitr-tools:8.0.46 image, Python 3 and rclone in PATH.
Works on Linux or Docker Desktop; the database/client always run on Linux.
"""
import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import secrets
import signal
import subprocess
import tempfile
from unittest import mock

from integration_archive import archive_real
import pitr_binlog

HERE = Path(__file__).resolve().parent
spec = importlib.util.spec_from_file_location("pitr_restore_integration", HERE / "mysql-pitr-restore.py")
restore = importlib.util.module_from_spec(spec)
spec.loader.exec_module(restore)


def snapshot(server, config, work, source_uuid):
    server.assert_owned()
    args = restore.backup.parse_args(["run", "--container", server.container_id, "--database", "pitr_fixture", "--pitr-anchor",
        "--remote", "crypt:fixture/full", "--rclone-config", str(config), "--state-dir", str(work / "backup-state"),
        "--release-lock", str(work / "release.lock"), "--job-lock", str(work / "backup.lock"), "--disk-path", str(work),
        "--min-free-bytes", "10485760", "--stop-free-bytes", "1048576"])
    # The only bypass is the Unix root requirement on the workstation. The
    # actual protected dump guard, DDL lease, pipeline, crypt and markers run.
    with mock.patch.object(restore.backup, "require_root"):
        receipt = restore.backup.execute(args)
    assert receipt["source"]["server_uuid"] == source_uuid
    return restore.coordinate(receipt["source"]["binlog_anchor"]), receipt["destination"]


def run(work):
    os.chdir(work)  # Local crypt backend is relative; no production path exists.
    config = work / "rclone.conf"
    obscured = subprocess.run(["rclone", "obscure", "--", secrets.token_urlsafe(32)], check=True, stdout=subprocess.PIPE).stdout.decode().strip()
    config.write_text("[local]\ntype=local\n[crypt]\ntype=crypt\nremote=local:objects\npassword=" + obscured + "\n")
    config.chmod(0o600)
    with restore.backup.Processes() as processes:
        remote = restore.backup.CryptRemote("crypt:fixture", config, processes)
        with restore.IsolatedMySQL(binlog=True) as source:
            assert source.query("SELECT JSON_ARRAY(@@max_allowed_packet)")[0][0] == 256 * 1024 * 1024
            source.exec(["mysql", "--user=root", "-e", "RESET MASTER; CREATE DATABASE pitr_fixture CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci;"])
            schema = """
            USE pitr_fixture;
            CREATE TABLE users(id BIGINT PRIMARY KEY,quota BIGINT,transferable_quota BIGINT,pulse_paid_quota BIGINT,used_quota BIGINT,request_count BIGINT,aff_quota BIGINT,aff_history BIGINT) ENGINE=InnoDB;
            CREATE TABLE top_ups(id BIGINT PRIMARY KEY,user_id BIGINT,trade_no VARCHAR(40),amount BIGINT,money DECIMAL(20,6),paid_money DECIMAL(20,6),presentment_money DECIMAL(20,6),paid_currency VARCHAR(8),presentment_currency VARCHAR(8),status VARCHAR(20),payment_provider VARCHAR(20),payment_method VARCHAR(20)) ENGINE=InnoDB;
            CREATE TABLE subscription_orders(id BIGINT PRIMARY KEY,trade_no VARCHAR(40),money DECIMAL(20,6),purchase_quantity INT,status VARCHAR(20)) ENGINE=InnoDB;
            CREATE TABLE logs(id BIGINT PRIMARY KEY,user_id BIGINT,type INT,quota BIGINT,prompt_tokens BIGINT,completion_tokens BIGINT,request_id VARCHAR(64),other LONGTEXT) ENGINE=InnoDB;
            CREATE TABLE pulse_wallet_reservations(id VARCHAR(64) PRIMARY KEY,request_id VARCHAR(64),user_id BIGINT,quota BIGINT,paid_quota BIGINT,status VARCHAR(20)) ENGINE=InnoDB;
            CREATE TABLE duplicate_rows(value VARBINARY(50)) ENGINE=InnoDB;
            CREATE TABLE large_rows(id BIGINT PRIMARY KEY,payload LONGBLOB NOT NULL) ENGINE=InnoDB;
            INSERT INTO users VALUES(1,1000,1000,1000,0,0,0,0);
            INSERT INTO duplicate_rows VALUES(0x0061090aff),(0x0061090aff),(NULL);
            INSERT INTO large_rows VALUES(1,REPEAT(CHAR(65),20*1024*1024));
            """
            source.exec(["mysql", "--user=root", "-e", schema])
            source_uuid = source.query("SELECT JSON_ARRAY(@@server_uuid)")[0][0]
            anchor_a, backup_a = snapshot(source, config, work, source_uuid)
            # Real InnoDB transactions: no payment provider or upstream call.
            source.exec(["mysql", "--user=root", "-e", """
            USE pitr_fixture; BEGIN;
            INSERT INTO top_ups VALUES(1,1,'synthetic-topup',200,2,2,2,'USD','USD','success','fixture','fixture');
            UPDATE users SET quota=quota+200,transferable_quota=transferable_quota+200,pulse_paid_quota=pulse_paid_quota+200 WHERE id=1;
            INSERT INTO logs VALUES(1,1,1,200,0,0,'topup','{}'); COMMIT;
            BEGIN; UPDATE users SET quota=quota-70,pulse_paid_quota=pulse_paid_quota-70,used_quota=used_quota+70,request_count=request_count+1 WHERE id=1;
            INSERT INTO logs VALUES(2,1,2,70,10,5,'consume','{}');
            INSERT INTO pulse_wallet_reservations VALUES('paid-1','consume',1,70,70,'settled'); COMMIT;
            FLUSH BINARY LOGS;
            BEGIN; UPDATE users SET quota=quota-30,pulse_paid_quota=pulse_paid_quota-30 WHERE id=1;
            INSERT INTO pulse_wallet_reservations VALUES('refund-1','refund',1,30,30,'reserved'); COMMIT;
            BEGIN; UPDATE users SET quota=quota+30,pulse_paid_quota=pulse_paid_quota+30 WHERE id=1;
            UPDATE pulse_wallet_reservations SET quota=0,paid_quota=0,status='refunded' WHERE id='refund-1';
            INSERT INTO logs VALUES(3,1,6,30,0,0,'refund','{}'); COMMIT;
            ALTER TABLE users ADD COLUMN audit_note VARBINARY(100) NULL;
            UPDATE users SET audit_note=0x0061090aff WHERE id=1;
            INSERT INTO subscription_orders VALUES(1,'synthetic-subscription',5,1,'success');
            INSERT INTO top_ups VALUES(2,1,'synthetic-subscription',0,5,5,5,'USD','USD','success','fixture','fixture');
            UPDATE large_rows SET payload=CONCAT(REPEAT(CHAR(66),20*1024*1024-1),UNHEX('FF')) WHERE id=1;
            """])
            assert source.query("SELECT JSON_ARRAY(OCTET_LENGTH(payload)) FROM pitr_fixture.large_rows")[0][0] == 20 * 1024 * 1024
            source.exec(["mysql", "--user=root", "-e", """
            CREATE DATABASE `newapi_preflight_20261023000000_12345` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
            CREATE USER 'pitr_preflight_fixture'@'%' IDENTIFIED BY 'disposable-test-password';
            GRANT ALL PRIVILEGES ON `newapi_preflight_20261023000000_12345`.* TO 'pitr_preflight_fixture'@'%';
            USE newapi_preflight_20261023000000_12345;
            CREATE TABLE fixture(id BIGINT PRIMARY KEY, amount DECIMAL(20,6));
            ALTER TABLE fixture ADD COLUMN note VARCHAR(100) DEFAULT 'nonbusiness';
            INSERT INTO fixture VALUES(1,10,'discarded');
            CREATE TABLE `_TMP_UNI_TOKENS_REF`(id BIGINT PRIMARY KEY) ENGINE=InnoDB;
            DROP TABLE IF EXISTS `_TMP_UNI_TOKENS_REF`;
            """])
            source.exec(["mysql", "--user=root", "-e", """
            DROP DATABASE IF EXISTS `newapi_preflight_20261023000000_12345`;
            DROP USER IF EXISTS 'pitr_preflight_fixture'@'%';
            """])
            anchor_b, backup_b = snapshot(source, config, work, source_uuid)
            # MySQL itself appends this comment to ordinary DROP TABLE binlog
            # queries. It is not present in the submitted SQL, and TEMPORARY
            # TABLE statements would not exercise this path with ROW logging.
            generated_drop = b"DROP TABLE IF EXISTS `_TMP_UNI_TOKENS_REF` /* generated by server */"
            binlog_sql = source.exec(["mysqlbinlog", "--verify-binlog-checksum", "--short-form",
                                     "/var/lib/mysql/" + anchor_b["file"]])
            assert generated_drop in binlog_sql.splitlines(), "official mysqlbinlog did not emit the server-generated DROP comment"
            # Transactions after T must not appear in the replay result.
            source.exec(["mysql", "--user=root", "-e", "USE pitr_fixture; INSERT INTO logs VALUES(4,1,2,999,999,999,'after-target','{}');"])
            archived = archive_real(source, remote, work, anchor_a["file"])
            segments, epoch = archived["segments"], archived["root"]
        args = restore.parse_args(["--backup-a", backup_a, "--backup-b", backup_b, "--epoch", epoch, "--rclone-config", str(config), "--work-dir", str(work / "restore")])
        result = restore.execute(args)
        detail = json.loads((work / "restore/result.json").read_bytes())
        assert detail["differences"] == []
        largest_event = 0
        generated_drop_events = 0
        for path in (work / "restore/binlogs").glob("binlog.*"):
            with path.open("rb") as stream:
                assert stream.read(4) == pitr_binlog.MAGIC
                while header := stream.read(pitr_binlog.HEADER.size):
                    _, kind, _, event_size, _, _ = pitr_binlog.HEADER.unpack(header)
                    largest_event = max(largest_event, event_size)
                    if kind == 2:
                        payload = stream.read(event_size - pitr_binlog.HEADER.size - 4)
                        stream.seek(4, 1)
                        if pitr_binlog.query_text(payload) == generated_drop.upper():
                            status_length = int.from_bytes(payload[11:13], "little")
                            default_db = payload[13 + status_length:13 + status_length + payload[8]]
                            assert default_db == b"newapi_preflight_20261023000000_12345"
                            generated_drop_events += 1
                    else:
                        stream.seek(event_size - pitr_binlog.HEADER.size, 1)
        assert generated_drop_events == 1
        assert largest_event > 40 * 1024 * 1024
        assert detail["replay_window"]["new_rows"]["logs"]["consume"] == 1
        assert detail["replay_window"]["new_rows"]["logs"]["refund"] == 1
        assert detail["incremental_audit"]["tables"]["logs"]["count"] == 3
        assert detail["incremental_audit"]["tables"]["large_rows"]["count"] == 1
        assert detail["incremental_audit"]["tables"]["top_ups"]["financial"]["wallet_only_rows"][0][-2:] == ["2.000000000000", "2.000000000000"]
        # Same real chain must reject an event-interior target before Docker.
        try:
            directory = work / "bad-stop"
            directory.mkdir(mode=0o700)
            restore.reconstruct(remote, epoch, directory, source_uuid, anchor_a,
                                dict(anchor_b, position=anchor_b["position"] - 1), "pitr_fixture")
        except restore.RestoreError:
            pass
        else:
            raise AssertionError("middle-of-event target was accepted")
        explicit = restore.parse_args(["--backup-a", backup_a, "--stop-file", anchor_b["file"], "--stop-position", str(anchor_b["position"]), "--export-restored-dump",
            "--epoch", epoch, "--rclone-config", str(config), "--work-dir", str(work / "explicit-restore")])
        assert restore.execute(explicit)["status"] == "success"
        explicit_detail = json.loads((work / "explicit-restore/result.json").read_bytes())
        assert restore.pitr_audit.compare_audits(explicit_detail["incremental_audit"], detail["reference_audit"]) == []
        exported = Path(explicit_detail["export"]["path"])
        assert exported.stat().st_mode & 0o777 == 0o600
        assert hashlib.sha256(exported.read_bytes()).hexdigest() == explicit_detail["export"]["sha256"]
        with restore.IsolatedMySQL() as exported_server:
            exported_server.load(exported)
            exported_audit = restore.pitr_audit.audit_database(exported_server.query, exported_server.lines, "pitr_fixture")
        assert restore.pitr_audit.compare_audits(exported_audit, detail["reference_audit"]) == []
        # Real SIGTERM exercises the same deadline handler used in production
        # recovery tools; cleanup is limited to this new labelled container.
        interrupted = restore.IsolatedMySQL()
        try:
            with restore.backup.deadline(120), interrupted:
                interrupted_id = interrupted.container_id
                os.kill(os.getpid(), signal.SIGTERM)
        except restore.backup.BackupError:
            pass
        else:
            raise AssertionError("SIGTERM was ignored")
        assert subprocess.run(["docker", "inspect", interrupted_id], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
        assert subprocess.run(["docker", "volume", "inspect", interrupted.volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode != 0
        return {"status": "success", "segments": segments, "tables": result["tables"], "anchor_a": anchor_a, "target_b": anchor_b,
                "large_row_bytes": 20 * 1024 * 1024,
                "largest_event_bytes": largest_event,
                "server_generated_drop_events": generated_drop_events,
                "coverage": ["real-mysql-8.0.46", "official-mysqlbinlog", "crypt-readback", "row-transactions", "topup", "consume", "refund", "ddl", "rotation", "split-transaction", "independent-full-comparison", "after-target-excluded", "binary-and-null", "duplicate-no-primary-key", "invalid-target-rejected", "explicit-position-cli", "SIGTERM-container-volume-cleanup", "production-backup-entry", "production-archive-entry", "archive-interruption-resume", "archive-idempotency", "release-preflight-scope", "server-generated-drop-comment", "restored-export-reimport", "20MiB-row-and-40MiB-update-event"], "work_dir": str(work)}


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work-dir", help="new absolute directory to retain synthetic evidence")
    args = parser.parse_args()
    if args.work_dir:
        destination = Path(args.work_dir).resolve()
        destination.mkdir(mode=0o700)
        print(json.dumps(run(destination), sort_keys=True))
    else:
        with tempfile.TemporaryDirectory(prefix="newapi-pitr-integration-") as temporary:
            print(json.dumps(run(Path(temporary).resolve()), sort_keys=True))
