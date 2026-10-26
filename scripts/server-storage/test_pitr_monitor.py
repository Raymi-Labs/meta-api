import datetime as dt
from contextlib import contextmanager
import hashlib
import importlib.util
import io
import json
import subprocess
import sys
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

    def unit_processes(self, **changes):
        values = {'LoadState': 'loaded', 'ActiveState': 'inactive', 'SubState': 'dead',
                  'Result': 'success', 'ExecMainStatus': '0'}
        values.update(changes)
        processes = MagicMock()
        child = processes.start.return_value
        child.returncode = 0
        child.communicate.return_value = ('\n'.join(key + '=' + value for key, value in values.items()).encode(), None)
        return processes

    def test_service_success_and_current_run_are_not_failures(self):
        for active, substate, prior_status in (('inactive', 'dead', '0'), ('active', 'running', '0'),
                                              ('active', 'exited', '0'), ('activating', 'start', '1'),
                                              ('activating', 'start-pre', '0'), ('activating', 'start-post', '0')):
            with self.subTest(active=active, substate=substate):
                processes = self.unit_processes(ActiveState=active, SubState=substate, ExecMainStatus=prior_status)
                result = m.check_unit('newapi-pitr-backup.service', processes)
                self.assertEqual(result['ActiveState'], active)
                command = processes.start.call_args.args[0]
                self.assertEqual(command[:3], ['systemctl', '--no-pager', 'show'])
                self.assertEqual(command[-2:], ['--', 'newapi-pitr-backup.service'])
                self.assertNotIn('is-enabled', command)

    def test_service_failure_signal_timeout_and_auto_restart_are_errors(self):
        cases = [{'Result': result} for result in ('exit-code', 'signal', 'timeout', 'core-dump', 'watchdog', 'oom-kill')]
        cases += [{'ActiveState': 'failed', 'SubState': 'failed'},
                  {'ActiveState': 'activating', 'SubState': 'auto-restart'},
                  {'ActiveState': 'activating', 'SubState': 'auto-restart-queued'},
                  {'ExecMainStatus': '15'}]
        for changes in cases:
            with self.subTest(changes=changes), self.assertRaises(m.ServiceFailure):
                m.check_unit('newapi-pitr-backup.service', self.unit_processes(**changes))

    def test_missing_unreadable_or_malformed_service_cannot_report_health(self):
        for state in ('not-found', 'masked', 'error'):
            with self.subTest(state=state), self.assertRaisesRegex(m.Error, 'not loaded') as caught:
                m.check_unit('missing.service', self.unit_processes(LoadState=state))
            self.assertNotIsInstance(caught.exception, m.ServiceFailure)
        processes = self.unit_processes()
        processes.start.return_value.returncode = 4
        with self.assertRaisesRegex(m.Error, 'unavailable') as caught:
            m.check_unit('missing.service', processes)
        self.assertNotIsInstance(caught.exception, m.ServiceFailure)
        processes = self.unit_processes()
        processes.start.return_value.communicate.side_effect = subprocess.TimeoutExpired('systemctl', 5)
        with self.assertRaisesRegex(m.Error, 'timed out') as caught:
            m.check_unit('missing.service', processes)
        self.assertNotIsInstance(caught.exception, m.ServiceFailure)
        for raw in (b'LoadState=loaded\n', b'not-properties', b'LoadState=loaded\nLoadState=loaded\n'):
            processes = self.unit_processes()
            processes.start.return_value.communicate.return_value = (raw, None)
            with self.subTest(raw=raw), self.assertRaisesRegex(m.Error, 'invalid') as caught:
                m.check_unit('newapi-pitr-backup.service', processes)
            self.assertNotIsInstance(caught.exception, m.ServiceFailure)

    def test_service_checks_are_explicit_repeatable_and_reject_non_service_names(self):
        self.assertEqual(m.parse_args([]).check_unit, [])
        self.assertEqual(m.parse_args(['--check-unit', 'backup.service', '--check-unit', 'archive.service']).check_unit,
                         ['backup.service', 'archive.service'])
        for value in ('../backup.service', 'backup.timer', '--all', 'bad name.service'):
            with self.subTest(value=value), patch('sys.stderr', new=io.StringIO()), self.assertRaises(SystemExit):
                m.parse_args(['--check-unit=' + value])

    def test_failed_backup_alerts_before_old_success_receipt_can_mask_it(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            config = root / 'alert.json'
            config.write_text('{}')
            config.chmod(0o600)
            latest = root / 'latest-success.json'
            latest.write_text(json.dumps({'status': 'success', 'finished_at': dt.datetime.now(dt.timezone.utc).isoformat()}))
            latest.chmod(0o600)
            args = m.parse_args(['--state-dir', str(root / 'state'), '--lock', str(root / 'monitor.lock'),
                                 '--full-backup-status', str(latest), '--alert-config', str(config),
                                 '--check-unit', 'newapi-pitr-backup.service'])
            with patch.object(m, 'check_unit', side_effect=m.ServiceFailure('PITR service failed: newapi-pitr-backup.service')) as check, \
                    patch.object(m, 'alert') as deliver, patch.object(m, 'source_position') as source:
                report = m.execute(args)
                check.assert_called_once()
                source.assert_not_called()
                deliver.assert_called_once()
                self.assertEqual(deliver.call_args.args[1], '[New API PITR FAILURE]')
                self.assertFalse(report['healthy'])
                self.assertIn('newapi-pitr-backup.service', report['error'])

    def test_main_returns_failure_for_unhealthy_report_and_monitor_errors(self):
        with patch('sys.stdout', new=io.StringIO()), patch('sys.stderr', new=io.StringIO()):
            for healthy, expected in ((True, 0), (False, 1)):
                with patch.object(m, 'execute', return_value={'healthy': healthy}):
                    self.assertEqual(m.main([]), expected)
            for error in (m.Error('delivery failed'), OSError('input failed'), ValueError('invalid input')):
                with patch.object(m, 'execute', side_effect=error):
                    self.assertEqual(m.main([]), 1)

    def test_cli_monitor_and_notification_failure_exits_nonzero(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            result = subprocess.run([sys.executable, '-B', str(Path(m.__file__).resolve()),
                '--state-dir', str(root / 'state'), '--lock', str(root / 'monitor.lock'),
                '--archive-status', str(root / 'missing-status.json'), '--alert-config', str(root / 'missing-alert.json')],
                stdout=subprocess.PIPE, stderr=subprocess.PIPE, timeout=10)
            self.assertEqual(result.returncode, 1)
            self.assertIn(b'PITR monitor failed', result.stderr)

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
                  'started_at': (self.now - dt.timedelta(minutes=20)).isoformat(),
                  'verified_at': (self.now - dt.timedelta(minutes=1)).isoformat(),
                  'finished_at': self.now.isoformat(),
                  'source': {'binlog_anchor': {'server_uuid': 'uuid', 'file': 'binlog.000001', 'position': 150},
                             'database_scope': ['new-api'], 'gtid_mode': 'OFF'},
                  'dump': {'name': 'mysql.sql.gz', 'size': 1234, 'sha256': 'dump-hash'}}
        manifest = m.backup.json_bytes({key: latest[key] for key in
            ('run_id', 'source', 'dump', 'started_at', 'verified_at')})
        marker = m.backup.json_bytes({'status': 'success', 'run_id': 'run1',
            'finished_at': latest['finished_at'], 'verified_at': latest['verified_at'],
            'manifest_sha256': hashlib.sha256(manifest).hexdigest()})
        status = {**self.status, 'root': 'crypt:pitr/uuid/epoch', 'verified_cursor': {'file': 'binlog.000001', 'position': 200}, 'transaction_open': False}
        remote = MagicMock()
        remote.name = 'crypt'
        child = remote.processes.start.return_value
        child.communicate.return_value = (m.backup.json_bytes({'IsDir': False, 'Size': 1234}), None)
        return root, latest, manifest, marker, status, remote

    @contextmanager
    def running_monitor(self, directory):
        root = Path(directory).resolve()
        archive_root, latest, _, _, status, remote = self.full_fixture()
        latest['finished_at'] = (self.now - dt.timedelta(seconds=30)).isoformat()
        args = m.parse_args(['--state-dir', str(root / 'state'), '--lock', str(root / 'monitor.lock'),
            '--archive-status', str(root / 'archive.json'), '--full-backup-status', str(root / 'latest.json'),
            '--alert-config', str(root / 'alert.json'), '--check-unit', m.FULL_BACKUP_UNIT])
        for path, value in ((args.archive_status, status), (args.full_backup_status, latest), (args.alert_config, {})):
            m.backup.atomic_json(Path(path), value)

        def read_remote(_remote, path, _limit):
            manifest = {key: latest[key] for key in ('run_id', 'source', 'dump', 'started_at', 'verified_at')}
            manifest_raw = m.backup.json_bytes(manifest)
            if path.endswith('/root.json'):
                return m.backup.json_bytes(archive_root)
            if path.endswith('/manifest.json'):
                return manifest_raw
            if path.endswith('/SUCCESS.json'):
                return m.backup.json_bytes({'status': 'success', 'run_id': latest['run_id'],
                    'finished_at': latest['finished_at'], 'verified_at': latest['verified_at'],
                    'manifest_sha256': hashlib.sha256(manifest_raw).hexdigest()})
            raise AssertionError('unexpected remote object')

        owner = self
        class Clock(dt.datetime):
            @classmethod
            def now(cls, tz=None):
                return owner.now

        with patch.object(m, 'dt', SimpleNamespace(datetime=Clock, timezone=dt.timezone)), \
                patch.object(m, 'check_unit', return_value={'ActiveState': 'inactive'}) as unit, \
                patch.object(m.backup, 'CryptRemote', return_value=remote), \
                patch.object(m, 'remote_bytes', side_effect=read_remote) as read, \
                patch.object(m, 'source_position', return_value=self.current), \
                patch.object(m, 'coverage', return_value=0), \
                patch.object(m, 'verify_tail', return_value='verified-tail'), \
                patch.object(m.shutil, 'disk_usage', return_value=SimpleNamespace(free=10 * 1024**3)), \
                patch.object(m, 'alert') as deliver:
            yield args, latest, unit, read, deliver

    def complete_new_backup(self, args, latest):
        self.now += dt.timedelta(minutes=5)
        latest.update(run_id='run2', destination='crypt:full/run2',
                      started_at=(self.now - dt.timedelta(minutes=4)).isoformat(),
                      verified_at=(self.now - dt.timedelta(seconds=10)).isoformat(),
                      finished_at=self.now.isoformat())
        m.backup.atomic_json(Path(args.full_backup_status), latest)

    def test_failed_backup_survives_restart_and_retry_until_verified_new_full(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, read, deliver = fixture
            diagnostic = 'PITR service failed: ' + m.FULL_BACKUP_UNIT + ' (result=exit-code, state=failed/failed)'
            unit.side_effect = m.ServiceFailure(diagnostic, {'Result': 'exit-code', 'ActiveState': 'failed'})
            self.assertFalse(m.execute(args)['healthy'])
            state = Path(args.state_dir)
            failure = json.loads((state / 'backup-failure.json').read_text())
            self.assertEqual(failure['previous_success']['run_id'], 'run1')
            self.assertEqual(failure['service_error'], diagnostic)
            self.assertEqual(failure['service_state']['Result'], 'exit-code')
            unit.side_effect = None
            # Each call reloads the disk state: no in-memory latch survives here.
            for active in ('activating', 'active', 'inactive'):
                unit.return_value = {'ActiveState': active, 'Result': 'success'}
                report = m.execute(args)
                self.assertFalse(report['healthy'])
                self.assertEqual(report['error'], m.BACKUP_PENDING)
                self.assertEqual(report['backup_service_error'], diagnostic)
                self.assertEqual(json.loads((state / 'backup-failure.json').read_text()), failure)
            self.assertEqual(deliver.call_count, 1, 'retrying must not send recovery or duplicate failure')
            self.complete_new_backup(args, latest)
            report = m.execute(args)
            self.assertTrue(report['healthy'])
            self.assertEqual(report['backup_failure_recovered_by'], 'run2')
            self.assertFalse((state / 'backup-failure.json').exists())
            self.assertEqual(deliver.call_args.args[1], '[New API PITR RECOVERED]')
            self.assertEqual(deliver.call_count, 2)
            self.assertGreater(read.call_count, 0)
            self.assertTrue(m.execute(args)['healthy'])
            self.assertEqual(deliver.call_count, 2, 'recovery is delivered only once')

    def test_first_normal_backup_running_does_not_create_failure_watermark(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, _, unit, _, deliver = fixture
            unit.return_value = {'ActiveState': 'activating', 'Result': 'success'}
            self.assertTrue(m.execute(args)['healthy'])
            self.assertFalse((Path(args.state_dir) / 'backup-failure.json').exists())
            deliver.assert_not_called()

    def test_service_query_errors_do_not_latch_a_running_backup_as_failed(self):
        for message in ('PITR service status query timed out', 'PITR service status is unavailable',
                        'PITR service status is invalid', 'PITR service is not loaded',
                        'backup exceeded its time limit', 'backup was interrupted'):
            with self.subTest(message=message), tempfile.TemporaryDirectory() as directory, \
                    self.running_monitor(directory) as fixture:
                args, latest, unit, _, _ = fixture
                started = self.now - dt.timedelta(minutes=1)
                unit.side_effect = m.Error(message)
                report = m.execute(args)
                self.assertFalse(report['healthy'])
                self.assertEqual(report['error'], message)
                self.assertFalse((Path(args.state_dir) / 'backup-failure.json').exists())
                unit.side_effect = None
                self.complete_new_backup(args, latest)
                latest['started_at'] = started.isoformat()
                m.backup.atomic_json(Path(args.full_backup_status), latest)
                self.assertTrue(m.execute(args)['healthy'])

    def test_query_error_does_not_advance_existing_failure_watermark(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, _, _ = fixture
            unit.side_effect = m.ServiceFailure('original confirmed backup failure')
            self.assertFalse(m.execute(args)['healthy'])
            path = Path(args.state_dir) / 'backup-failure.json'
            original = path.read_bytes()
            retry_started = self.now + dt.timedelta(minutes=1)
            self.now += dt.timedelta(minutes=2)
            unit.side_effect = m.Error('PITR service status query timed out')
            report = m.execute(args)
            self.assertFalse(report['healthy'])
            self.assertEqual(path.read_bytes(), original)
            self.assertEqual(report['backup_service_error'], 'original confirmed backup failure')
            unit.side_effect = None
            self.complete_new_backup(args, latest)
            latest['started_at'] = retry_started.isoformat()
            m.backup.atomic_json(Path(args.full_backup_status), latest)
            self.assertTrue(m.execute(args)['healthy'])
            self.assertFalse(path.exists())

    def test_pending_failure_reminder_retains_original_service_diagnostics(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, _, unit, _, deliver = fixture
            diagnostic = 'PITR service failed: newapi-pitr-backup.service (result=timeout, state=failed/failed)'
            unit.side_effect = m.ServiceFailure(diagnostic, {'Result': 'timeout', 'ActiveState': 'failed'})
            self.assertFalse(m.execute(args)['healthy'])
            unit.side_effect = None
            self.now += dt.timedelta(seconds=args.reminder_seconds + 1)
            self.assertFalse(m.execute(args)['healthy'])
            self.assertEqual(deliver.call_count, 2)
            report = deliver.call_args.args[2]
            self.assertEqual(report['backup_service_error'], diagnostic)
            self.assertEqual(report['backup_failure']['service_state']['Result'], 'timeout')

    def test_clearing_failure_persists_directory_deletion(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory).resolve() / 'backup-failure.json'
            m.backup.atomic_json(path, {'fixture': True})
            original_fsync = m.os.fsync
            observed = []
            def fsync_after_unlink(fd):
                observed.append(path.exists())
                original_fsync(fd)
            with patch.object(m.os, 'fsync', side_effect=fsync_after_unlink):
                m.clear_backup_failure(path)
            self.assertEqual(observed, [False])
            self.assertFalse(path.exists())

    def test_failure_without_previous_success_is_latched_and_can_recover(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, _, deliver = fixture
            Path(args.full_backup_status).unlink()
            unit.side_effect = m.ServiceFailure('service failed')
            self.assertFalse(m.execute(args)['healthy'])
            path = Path(args.state_dir) / 'backup-failure.json'
            self.assertIsNone(json.loads(path.read_text())['previous_success'])
            unit.side_effect = None
            self.complete_new_backup(args, latest)
            self.assertTrue(m.execute(args)['healthy'])
            self.assertEqual(deliver.call_args.args[1], '[New API PITR RECOVERED]')

    def test_later_failure_invalidates_a_backup_started_before_that_failure(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, _, _ = fixture
            unit.side_effect = m.ServiceFailure('service failed')
            m.execute(args)
            self.now += dt.timedelta(minutes=2)
            m.execute(args)
            unit.side_effect = None
            failed_at = self.now
            self.complete_new_backup(args, latest)
            latest['started_at'] = (failed_at - dt.timedelta(seconds=1)).isoformat()
            m.backup.atomic_json(Path(args.full_backup_status), latest)
            self.assertEqual(m.execute(args)['error'], m.BACKUP_PENDING)
            self.assertTrue((Path(args.state_dir) / 'backup-failure.json').exists())

    def test_remote_verification_failure_does_not_clear_backup_failure(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, read, _ = fixture
            unit.side_effect = m.ServiceFailure('service failed')
            m.execute(args)
            unit.side_effect = None
            self.complete_new_backup(args, latest)
            read.side_effect = m.Error('remote full backup manifest differs from its completion receipt')
            self.assertFalse(m.execute(args)['healthy'])
            self.assertTrue((Path(args.state_dir) / 'backup-failure.json').exists())

    def test_final_rpo_failure_does_not_clear_backup_failure(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, _, _ = fixture
            unit.side_effect = m.ServiceFailure('service failed')
            m.execute(args)
            unit.side_effect = None
            self.complete_new_backup(args, latest)
            with patch.object(m, 'coverage', side_effect=[0, m.Error('RPO limit exceeded')]):
                self.assertFalse(m.execute(args)['healthy'])
            self.assertTrue((Path(args.state_dir) / 'backup-failure.json').exists())

    def test_old_run_or_invalid_time_order_cannot_clear_failure(self):
        for change in ('same-run', 'old-start', 'invalid-order'):
            with self.subTest(change=change), tempfile.TemporaryDirectory() as directory, \
                    self.running_monitor(directory) as fixture:
                args, latest, unit, _, _ = fixture
                unit.side_effect = m.ServiceFailure('service failed')
                m.execute(args)
                unit.side_effect = None
                observed = self.now
                self.complete_new_backup(args, latest)
                if change == 'same-run':
                    latest['run_id'] = 'run1'
                elif change == 'old-start':
                    latest['started_at'] = observed.isoformat()
                else:
                    latest['verified_at'] = (self.now + dt.timedelta(seconds=1)).isoformat()
                m.backup.atomic_json(Path(args.full_backup_status), latest)
                self.assertEqual(m.execute(args)['error'], m.BACKUP_PENDING)
                self.assertTrue((Path(args.state_dir) / 'backup-failure.json').exists())

    def test_concurrent_success_is_not_recorded_as_failed_runs_old_baseline(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory).resolve()
            latest_path, path = root / 'latest.json', root / 'failure.json'
            m.backup.atomic_json(latest_path, {'status': 'success', 'run_id': 'concurrent-retry',
                'finished_at': (self.now + dt.timedelta(seconds=1)).isoformat()})
            value = m.remember_backup_failure(path, latest_path, self.now, None, m.ServiceFailure('service failed'))
            self.assertIsNone(value['previous_success'])
            self.assertEqual(m.backup_failure(path, self.now), value)

    def test_corrupt_backup_failure_watermark_cannot_report_recovery(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, _, _, _, deliver = fixture
            state = m.backup.private_directory(args.state_dir)
            m.backup.atomic_json(state / 'backup-failure.json', {'format_version': 1})
            report = m.execute(args)
            self.assertFalse(report['healthy'])
            self.assertEqual(deliver.call_args.args[1], '[New API PITR FAILURE]')
            self.assertTrue((state / 'backup-failure.json').exists())

    def test_recovery_mail_failure_retains_notification_for_next_attempt(self):
        with tempfile.TemporaryDirectory() as directory, self.running_monitor(directory) as fixture:
            args, latest, unit, _, deliver = fixture
            unit.side_effect = m.ServiceFailure('service failed')
            m.execute(args)
            unit.side_effect = None
            self.complete_new_backup(args, latest)
            deliver.side_effect = m.Error('mail unavailable')
            with self.assertRaisesRegex(m.Error, 'notification failed'):
                m.execute(args)
            notification = json.loads((Path(args.state_dir) / 'notification.json').read_text())
            self.assertEqual(notification['error'], m.BACKUP_PENDING)
            deliver.side_effect = None
            self.assertTrue(m.execute(args)['healthy'])
            self.assertEqual(deliver.call_args.args[1], '[New API PITR RECOVERED]')

    def test_full_backup_times_must_match_verified_remote_metadata(self):
        for key in ('started_at', 'verified_at', 'finished_at'):
            root, latest, manifest, marker, status, remote = self.full_fixture()
            latest[key] = (self.now + dt.timedelta(seconds=1)).isoformat()
            with self.subTest(key=key), patch.object(m, 'remote_bytes', side_effect=[m.backup.json_bytes(root), marker, manifest]), \
                    self.assertRaisesRegex(m.Error, 'differs'):
                m.verify_full_backup(remote, status, latest, self.current, self.now, 3600)

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
