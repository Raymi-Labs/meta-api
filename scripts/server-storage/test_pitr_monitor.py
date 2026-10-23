import datetime as dt
import hashlib
import importlib.util
import io
import json
import tempfile
from pathlib import Path
from types import SimpleNamespace
import unittest
from unittest.mock import MagicMock, patch

_spec = importlib.util.spec_from_file_location('pitr_monitor', Path(__file__).with_name('pitr-monitor.py'))
m = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(m)


class MonitorTests(unittest.TestCase):
    def setUp(self):
        self.now = dt.datetime(2026, 10, 23, tzinfo=dt.timezone.utc)
        self.status = {'status': 'ok', 'source_uuid': 'uuid',
            'last_observed_at': self.now.isoformat(),
            'covered_sample_at': (self.now-dt.timedelta(seconds=120)).isoformat(),
            'last_complete_transaction_frontier': {'file': 'binlog.000001', 'position': 200}}
        self.current = {'source_uuid': 'uuid', 'file': 'binlog.000001', 'position': 300}

    def test_backlog_uses_covered_sampling_time(self):
        self.assertEqual(m.coverage(self.status, self.current, self.now, 300, 180), 120)

    def test_idle_equal_position_is_not_stale_transaction_alarm(self):
        self.current['position'] = 200
        self.status['covered_sample_at'] = '2026-01-01T00:00:00+00:00'
        self.assertEqual(m.coverage(self.status, self.current, self.now, 300, 180), 0)

    def test_old_worker_cannot_claim_idle_health(self):
        self.current['position'] = 200
        self.status['last_observed_at'] = '2026-01-01T00:00:00+00:00'
        with self.assertRaisesRegex(m.Error, 'stale'):
            m.coverage(self.status, self.current, self.now, 300, 180)

    def test_gap_uuid_reset_and_rpo_fail_closed(self):
        for change in ({'source_uuid': 'another'}, {'file': 'binlog.000000'}, {'position': 100}):
            with self.subTest(change=change), self.assertRaises(m.Error):
                m.coverage(self.status, {**self.current, **change}, self.now, 300, 180)
        with self.assertRaisesRegex(m.Error, 'RPO'):
            m.coverage(self.status, self.current, self.now + dt.timedelta(seconds=200), 300, 400)

    def test_error_or_uninitialized_worker_fails(self):
        for state in ['error', 'initializing']:
            with self.subTest(state=state), self.assertRaises(m.Error):
                m.coverage({**self.status, 'status': state}, self.current, self.now, 300, 180)

    def test_future_timestamp_rejected(self):
        with self.assertRaises(m.Error):
            m.age((self.now + dt.timedelta(minutes=2)).isoformat(), self.now)

    def test_notification_transition_dedup_reminder_and_recovery(self):
        due, signature = m.notification_due({}, 'failure', self.now, 3600)
        self.assertTrue(due)
        previous = {'signature': signature, 'error': 'failure', 'sent_at': self.now.isoformat()}
        self.assertFalse(m.notification_due(previous, 'failure', self.now, 3600)[0])
        self.assertTrue(m.notification_due(previous, 'failure', self.now + dt.timedelta(hours=1), 3600)[0])
        self.assertTrue(m.notification_due(previous, None, self.now, 3600)[0])
        self.assertFalse(m.notification_due({}, None, self.now, 3600)[0])

    def test_smtp_delivery_uses_tls_and_does_not_leak_failure(self):
        config = {'transport': 'smtp', 'host': 'smtp.test', 'port': 587, 'tls': 'starttls',
                  'username': 'account', 'password': 'secret-value', 'from': 'from@test', 'to': ['to@test']}
        with patch.object(m.smtplib, 'SMTP') as client:
            smtp = client.return_value.__enter__.return_value
            smtp.send_message.return_value = {}
            m.alert(config, 'test', {'healthy': False})
            smtp.starttls.assert_called_once()
            smtp.login.assert_called_once_with('account', 'secret-value')
            smtp.login.side_effect = ValueError('secret-value')
            with self.assertRaises(m.Error) as caught:
                m.alert(config, 'test', {})
            self.assertNotIn('secret-value', str(caught.exception))

    def test_no_plaintext_smtp_or_http_webhook(self):
        for config in [{'transport': 'smtp', 'tls': 'none'}, {'transport': 'webhook', 'url': 'http://test'}]:
            with self.subTest(config=config), self.assertRaises(m.Error):
                m.alert(config, 'test', {})

    def test_remote_receipt_and_content_are_rechecked(self):
        root = 'crypt:pitr/uuid/epoch'
        data = b'verified segment'
        receipt = {'source_uuid': 'uuid', 'last_complete_transaction_frontier': self.status['last_complete_transaction_frontier'],
            'object': 'segments/000000000001-binlog.000001-0-16-hash.bin',
            'size': len(data), 'sha256': hashlib.sha256(data).hexdigest()}
        raw = m.backup.json_bytes(receipt)
        status = {**self.status, 'root': root, 'latest_receipt_object': root+'/receipts/000000000001.json',
                  'latest_receipt_sha256': hashlib.sha256(raw).hexdigest()}
        remote = MagicMock()
        remote.reader.side_effect = [SimpleNamespace(stdout=io.BytesIO(raw)), SimpleNamespace(stdout=io.BytesIO(data))]
        self.assertEqual(m.verify_tail(remote, status), status['latest_receipt_object'])
        remote.reader.side_effect = [SimpleNamespace(stdout=io.BytesIO(raw)), SimpleNamespace(stdout=io.BytesIO(b'corrupted'))]
        with self.assertRaisesRegex(m.Error, 'verification failed'):
            m.verify_tail(remote, status)

    def test_remote_receipt_cannot_redirect_to_other_path(self):
        status = {**self.status, 'root': 'crypt:pitr/uuid/epoch', 'latest_receipt_object': 'crypt:another/file'}
        with self.assertRaises(m.Error):
            m.verify_tail(MagicMock(), status)

    def test_remote_frontier_cannot_differ_from_local_checkpoint(self):
        root = 'crypt:pitr/uuid/epoch'
        raw = m.backup.json_bytes({'source_uuid': 'uuid', 'last_complete_transaction_frontier': {'file': 'binlog.000001', 'position': 100}})
        status = {**self.status, 'root': root, 'latest_receipt_object': root+'/receipts/000000000001.json', 'latest_receipt_sha256': hashlib.sha256(raw).hexdigest()}
        remote = MagicMock()
        remote.reader.return_value.stdout = io.BytesIO(raw)
        with self.assertRaisesRegex(m.Error, 'frontier differs'):
            m.verify_tail(remote, status)

    def full_fixture(self):
        root = {'uuid': 'uuid', 'start_file': 'binlog.000001', 'database_scope': ['new-api'], 'gtid_mode': 'OFF'}
        latest = {'status': 'success', 'run_id': 'run1', 'destination': 'crypt:full/run1',
                  'finished_at': self.now.isoformat(),
                  'source': {'binlog_anchor': {'server_uuid': 'uuid', 'file': 'binlog.000001', 'position': 150},
                             'database_scope': ['new-api'], 'gtid_mode': 'OFF'},
                  'dump': {'name': 'mysql.sql.gz', 'size': 1234, 'sha256': 'dump-hash'}}
        manifest = m.backup.json_bytes({'run_id': 'run1', 'source': latest['source'], 'dump': latest['dump']})
        marker = m.backup.json_bytes({'status': 'success', 'run_id': 'run1', 'manifest_sha256': hashlib.sha256(manifest).hexdigest()})
        status = {**self.status, 'root': 'crypt:pitr/uuid/epoch', 'verified_cursor': {'file': 'binlog.000001', 'position': 200}, 'transaction_open': False}
        remote = MagicMock()
        remote.name = 'crypt'
        child = remote.processes.start.return_value
        child.communicate.return_value = (m.backup.json_bytes({'IsDir': False, 'Size': 1234}), None)
        return root, latest, manifest, marker, status, remote

    def test_full_backup_remote_markers_and_object_size_checked(self):
        root, latest, manifest, marker, status, remote = self.full_fixture()
        with patch.object(m, 'remote_bytes', side_effect=[m.backup.json_bytes(root), marker, manifest]):
            self.assertEqual(m.verify_full_backup(remote, status, latest, self.current, self.now, 3600), 'crypt:full/run1')
        remote.processes.start.return_value.communicate.return_value = (m.backup.json_bytes({'IsDir': False, 'Size': 1233}), None)
        with patch.object(m, 'remote_bytes', side_effect=[m.backup.json_bytes(root), marker, manifest]), self.assertRaisesRegex(m.Error, 'size differs'):
            m.verify_full_backup(remote, status, latest, self.current, self.now, 3600)

    def test_new_epoch_cannot_claim_older_full_backup_coverage(self):
        root, latest, manifest, marker, status, remote = self.full_fixture()
        root['start_file'] = 'binlog.000002'
        status['verified_cursor'] = {'file': 'binlog.000003', 'position': 200}
        with patch.object(m, 'remote_bytes', return_value=m.backup.json_bytes(root)), self.assertRaisesRegex(m.Error, 'outside'):
            m.verify_full_backup(remote, status, latest, self.current, self.now, 3600)

    def test_missing_or_changed_full_backup_marker_fails(self):
        root, latest, manifest, marker, status, remote = self.full_fixture()
        marker = m.backup.json_bytes({'status': 'success', 'run_id': 'other', 'manifest_sha256': hashlib.sha256(manifest).hexdigest()})
        with patch.object(m, 'remote_bytes', side_effect=[m.backup.json_bytes(root), marker, manifest]), self.assertRaisesRegex(m.Error, 'differs'):
            m.verify_full_backup(remote, status, latest, self.current, self.now, 3600)

    def test_malformed_input_is_a_fixed_diagnostic(self):
        for value in (b'[]', b'null', b'"private-token"', b'invalid'):
            with self.assertRaisesRegex(m.Error, '^invalid fixture$'):
                m.object_json(value, 'invalid fixture')
        for previous in ({'signature': 'bad', 'sent_at': self.now.isoformat()}, {'signature': 'f'*64, 'sent_at': 'secret-bad-date'}):
            with self.assertRaises(m.Error):
                m.notification_due(previous, 'failure', self.now, 3600)

    def test_bad_status_and_corrupt_dedup_state_still_send_failure(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            state = root / 'state'
            m.backup.private_directory(state)
            archive = root / 'archive.json'
            archive.write_text('[]')
            archive.chmod(0o600)
            notification = state / 'notification.json'
            notification.write_text('null')
            notification.chmod(0o600)
            config = root / 'alert.json'
            config.write_text('{}')
            config.chmod(0o600)
            args = m.parse_args(['--state-dir', str(state), '--lock', str(root / 'monitor.lock'),
                                 '--archive-status', str(archive), '--alert-config', str(config)])
            with patch.object(m, 'alert') as deliver:
                report = m.execute(args)
                deliver.assert_called_once()
                self.assertEqual(deliver.call_args.args[1], '[New API PITR FAILURE]')
            self.assertFalse(report['healthy'])
            self.assertEqual(report['error'], 'archive status is invalid')
            self.assertEqual(json.loads(notification.read_text())['error'], report['error'])


if __name__ == '__main__':
    unittest.main()
