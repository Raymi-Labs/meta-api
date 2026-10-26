#!/usr/bin/env python3
"""Independently verify PITR coverage and send deduplicated private alerts."""
import argparse
import datetime as dt
from email.message import EmailMessage
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import re
import shutil
import smtplib
import ssl
import subprocess
import sys
import urllib.request

_spec = importlib.util.spec_from_file_location('pitr_backup_common', Path(__file__).with_name('mysql-r2-backup.py'))
backup = importlib.util.module_from_spec(_spec)
_spec.loader.exec_module(backup)
Error = backup.BackupError
FULL_BACKUP_UNIT = 'newapi-pitr-backup.service'
BACKUP_PENDING = 'PITR full backup failure requires a newer verified backup: ' + FULL_BACKUP_UNIT


class ServiceFailure(Error):
    """A parsed service result confirms failure, rather than a failed query."""
    def __init__(self, message, state=None):
        super().__init__(message)
        self.state = dict(state or {})


def object_json(data, diagnostic):
    try:
        value = json.loads(data)
        if not isinstance(value, dict):
            raise ValueError()
        return value
    except (UnicodeError, ValueError, TypeError):
        raise Error(diagnostic) from None


def age(value, now):
    try:
        parsed = dt.datetime.fromisoformat(value.replace('Z', '+00:00'))
        if parsed.tzinfo is None:
            raise ValueError()
        result = (now - parsed).total_seconds()
        if result < -30:
            raise ValueError()
        return max(0, result)
    except (AttributeError, TypeError, ValueError):
        raise Error('invalid or future coverage timestamp') from None


def backup_failure(path, now):
    if not path.exists():
        return None
    value = object_json(backup.private_bytes(path), 'full backup failure watermark is invalid')
    if (value.get('format_version') != 1 or value.get('unit') != FULL_BACKUP_UNIT
            or 'previous_success' not in value
            or not isinstance(value.get('previous_success'), (dict, type(None)))
            or 'service_error' in value and (not isinstance(value['service_error'], str)
                                             or not value['service_error'] or len(value['service_error']) > 512)
            or 'service_state' in value and (not isinstance(value['service_state'], dict)
                                             or not all(isinstance(key, str) and isinstance(item, str)
                                                        for key, item in value['service_state'].items()))):
        raise Error('full backup failure watermark is invalid')
    age(value.get('observed_at'), now)
    previous = value['previous_success']
    if previous is not None:
        if not isinstance(previous.get('run_id'), str) or not previous['run_id']:
            raise Error('full backup failure watermark is invalid')
        age(previous.get('finished_at'), now)
    return value


def remember_backup_failure(path, latest_path, now, previous, error):
    # Snapshot the last success without allowing a concurrently completed retry
    # (after this failure observation) to become the baseline it must surpass.
    baseline = previous.get('previous_success') if previous else None
    try:
        latest = object_json(backup.private_bytes(latest_path), 'full backup receipt is invalid')
        age(latest.get('finished_at'), now)
        if (latest.get('status') == 'success' and isinstance(latest.get('run_id'), str) and latest['run_id']
                and dt.datetime.fromisoformat(latest['finished_at'].replace('Z', '+00:00')) <= now):
            baseline = {key: latest[key] for key in ('run_id', 'finished_at')}
    except (Error, OSError, ValueError, TypeError):
        pass  # A missing/invalid old success must not prevent latching failure.
    value = {'format_version': 1, 'unit': FULL_BACKUP_UNIT, 'observed_at': now.isoformat(),
             'previous_success': baseline, 'service_error': str(error), 'service_state': error.state}
    backup.atomic_json(path, value)
    return value


def clear_backup_failure(path):
    # The monitor lock serializes writers. Persist the deletion itself before
    # reporting recovery, independently of later health/notification writes.
    directory_fd = os.open(path.parent, os.O_RDONLY | os.O_DIRECTORY | os.O_CLOEXEC)
    try:
        path.unlink()
        os.fsync(directory_fd)
    finally:
        os.close(directory_fd)


def newer_backup_completed(failure, latest, now):
    """Call only after the remote marker and manifest have verified this receipt."""
    for key in ('started_at', 'verified_at', 'finished_at'):
        age(latest.get(key), now)
    started, verified, finished = (dt.datetime.fromisoformat(latest[key].replace('Z', '+00:00'))
                                   for key in ('started_at', 'verified_at', 'finished_at'))
    observed = dt.datetime.fromisoformat(failure['observed_at'].replace('Z', '+00:00'))
    previous = failure['previous_success']
    if (not isinstance(latest.get('run_id'), str) or not latest['run_id']
            or not observed < started <= verified <= finished
            or previous and (latest['run_id'] == previous['run_id']
                             or finished <= dt.datetime.fromisoformat(previous['finished_at'].replace('Z', '+00:00')))):
        raise Error(BACKUP_PENDING)


def position(value):
    if (not isinstance(value, dict) or not re.fullmatch(r'[A-Za-z0-9_-][A-Za-z0-9_.-]*\.[0-9]{6,}', str(value.get('file', '')))
            or type(value.get('position')) is not int or value['position'] < 4):
        raise Error('invalid binlog position')
    prefix, number = value['file'].rsplit('.', 1)
    return prefix, int(number), value['position']


def coverage(status, current, now, max_lag, max_stale):
    if status.get('status') != 'ok':
        raise Error('archive worker has not reported a successful observation')
    if age(status.get('last_observed_at'), now) > max_stale:
        raise Error('archive worker observation is stale')
    if status.get('source_uuid') != current.get('source_uuid'):
        raise Error('source UUID differs from archive')
    # A fully verified idle FDE/ROTATE tail can cover a sample without inventing
    # a new committed transaction. Partial transactions still use the last XID.
    covered = (status.get('verified_cursor') if status.get('transaction_open') is False
               else status.get('last_complete_transaction_frontier'))
    source, frontier = position(current), position(covered)
    if source[0] != frontier[0] or source < frontier:
        raise Error('source binlog has reset or moved behind verified coverage')
    lag = 0 if source == frontier else age(status.get('covered_sample_at'), now)
    if lag > max_lag:
        raise Error('verified remote transaction coverage exceeds the RPO limit')
    return lag


def source_position(args, processes):
    sql = 'SELECT @@server_uuid; SHOW MASTER STATUS;'
    raw = processes.capture(backup.mysql_command(args, 'mysql', ['--protocol=SOCKET', '--batch', '--skip-column-names', '--execute=' + sql]))
    try:
        lines = raw.decode('ascii').splitlines()
        fields = lines[1].split('\t')
        result = {'source_uuid': lines[0], 'file': fields[0], 'position': int(fields[1])}
        position(result)
        return result
    except (UnicodeError, ValueError, IndexError):
        raise Error('source binlog status is unavailable') from None


def check_unit(unit, processes):
    """Read the latest service result without starting it or inspecting timers."""
    fields = ('LoadState', 'ActiveState', 'SubState', 'Result', 'ExecMainStatus')
    child = processes.start(['systemctl', '--no-pager', 'show',
                             '--property=' + ','.join(fields), '--', unit],
                            stdin=subprocess.DEVNULL, stdout=subprocess.PIPE)
    try:
        raw, _ = child.communicate(timeout=5)
    except subprocess.TimeoutExpired:
        raise Error('PITR service status query timed out: ' + unit) from None
    if child.returncode:
        raise Error('PITR service status is unavailable: ' + unit)
    try:
        values = {}
        for line in raw.decode('ascii').splitlines():
            key, value = line.split('=', 1)
            if key not in fields or key in values:
                raise ValueError()
            values[key] = value
        if values.get('LoadState') != 'loaded':
            raise Error('PITR service is not loaded: ' + unit)
        if (set(values) != set(fields)
                or any(not re.fullmatch(r'[a-z][a-z-]*', values[key]) for key in fields[:-1])
                or not re.fullmatch(r'[0-9]+', values['ExecMainStatus'])):
            raise ValueError()
        exit_status = int(values['ExecMainStatus'])
    except (UnicodeError, ValueError, KeyError):
        raise Error('PITR service status is invalid: ' + unit) from None
    active, substate, result = values['ActiveState'], values['SubState'], values['Result']
    if (active == 'failed' or substate == 'failed' or substate.startswith('auto-restart')
            or result != 'success'):
        raise ServiceFailure('PITR service failed: ' + unit + ' (result=' + result
                             + ', state=' + active + '/' + substate + ')', values)
    if active not in ('active', 'activating', 'reloading', 'deactivating', 'inactive'):
        raise Error('PITR service status is invalid: ' + unit)
    # ExecMainStatus can describe the previous main process while a new start
    # is in progress. Result/SubState still expose failed automatic retries.
    if active == 'inactive' and exit_status != 0:
        raise ServiceFailure('PITR service exited unsuccessfully: ' + unit, values)
    return values


def remote_bytes(remote, path, maximum):
    reader = remote.reader(path)
    data = reader.stdout.read(maximum + 1)
    if len(data) > maximum:
        raise Error('remote verification object exceeds its expected size')
    remote.processes.check(reader, 'remote verification read failed')
    return data


def verify_tail(remote, status):
    root = status.get('root', '')
    backup.remote_prefix(root)
    receipt_path = status.get('latest_receipt_object', '')
    if not re.fullmatch(re.escape(root) + r'/receipts/[0-9]{12}\.json', receipt_path):
        raise Error('invalid remote receipt reference')
    raw = remote_bytes(remote, receipt_path, backup.BLOCK)
    if hashlib.sha256(raw).hexdigest() != status.get('latest_receipt_sha256'):
        raise Error('remote receipt differs from verified checkpoint')
    receipt = object_json(raw, 'remote receipt is invalid')
    if (receipt.get('source_uuid') != status.get('source_uuid')
            or receipt.get('last_complete_transaction_frontier') != status.get('last_complete_transaction_frontier')):
        raise Error('remote receipt transaction frontier differs from checkpoint')
    parser = receipt.get('parser_state', {})
    if not isinstance(parser, dict):
        raise Error('remote parser state is invalid')
    if status.get('verified_cursor') is not None:
        if ({'file': parser.get('file'), 'position': parser.get('next_position')} != status['verified_cursor']
                or bool(parser.get('in_transaction') or parser.get('pending_gtid')) != status.get('transaction_open')):
            raise Error('remote parser coverage differs from checkpoint')
    obj = receipt.get('object', '')
    if not isinstance(obj, str):
        raise Error('invalid remote segment reference')
    if obj.startswith('segments/'):
        obj = root + '/' + obj
    if not re.fullmatch(re.escape(root) + r'/segments/[A-Za-z0-9_.-]+\.bin', obj):
        raise Error('invalid remote segment reference')
    size = receipt.get('size')
    if type(size) is not int or not 0 < size <= 256 * 1024**2:
        raise Error('invalid remote segment size')
    reader = remote.reader(obj)
    digest, received = hashlib.sha256(), 0
    while True:
        data = reader.stdout.read(backup.BLOCK)
        if not data:
            break
        received += len(data)
        if received > size:
            raise Error('remote segment is larger than its verified receipt')
        digest.update(data)
    remote.processes.check(reader, 'remote segment is missing or unreadable')
    if received != size or digest.hexdigest() != receipt.get('sha256'):
        raise Error('remote segment content verification failed')
    return receipt_path


def verify_full_backup(remote, status, latest, current, now, maximum_age):
    source = latest.get('source')
    if (latest.get('status') != 'success' or not isinstance(source, dict)
            or not isinstance(source.get('binlog_anchor'), dict)
            or age(latest.get('finished_at'), now) > maximum_age):
        raise Error('latest anchored full backup is missing or too old')
    anchor = source['binlog_anchor']
    if anchor.get('server_uuid') != current['source_uuid']:
        raise Error('latest full backup belongs to another MySQL source')
    root = object_json(remote_bytes(remote, status['root'] + '/root.json', backup.BLOCK), 'remote archive root is invalid')
    start = {'file': root.get('start_file'), 'position': 4}
    covered = (status.get('verified_cursor') if status.get('transaction_open') is False
               else status.get('last_complete_transaction_frontier'))
    if (root.get('uuid') != current['source_uuid'] or root.get('database_scope') != source.get('database_scope')
            or root.get('gtid_mode') != source.get('gtid_mode') or not source.get('database_scope')
            or position(start)[0] != position(anchor)[0]
            or position(anchor)[0] != position(covered)[0]
            or not position(start) <= position(anchor) <= position(covered)):
        raise Error('full backup anchor or database scope is outside the remote archive chain')
    destination, name = backup.remote_prefix(latest.get('destination', ''))
    if name != remote.name:
        raise Error('full backup destination is outside the configured encrypted remote')
    marker = object_json(remote_bytes(remote, destination + '/SUCCESS.json', backup.BLOCK), 'remote full backup completion marker is invalid')
    manifest_raw = remote_bytes(remote, destination + '/manifest.json', backup.BLOCK)
    manifest = object_json(manifest_raw, 'remote full backup manifest is invalid')
    if (marker.get('status') != 'success' or marker.get('run_id') != latest.get('run_id')
            or manifest.get('run_id') != latest.get('run_id')
            or marker.get('manifest_sha256') != hashlib.sha256(manifest_raw).hexdigest()
            or manifest.get('source') != source or manifest.get('dump') != latest.get('dump')
            or marker.get('finished_at') != latest.get('finished_at')
            or marker.get('verified_at') != latest.get('verified_at')
            or manifest.get('verified_at') != latest.get('verified_at')
            or manifest.get('started_at') != latest.get('started_at')):
        raise Error('remote full backup manifest differs from its completion receipt')
    dump = manifest.get('dump')
    if not isinstance(dump, dict) or dump.get('name') != 'mysql.sql.gz' or type(dump.get('size')) is not int or dump['size'] <= 0:
        raise Error('remote full backup dump metadata is invalid')
    child = remote.processes.start(remote.command('lsjson', destination + '/mysql.sql.gz', '--stat'),
                                   stdin=subprocess.DEVNULL, stdout=subprocess.PIPE, env=remote.env)
    raw, _ = child.communicate()
    remote.processes.check(child, 'remote full backup dump is missing or inaccessible')
    info = object_json(raw, 'remote full backup object metadata is invalid')
    if info.get('IsDir') is not False or info.get('Size') != dump['size']:
        raise Error('remote full backup dump size differs from its verified manifest')
    return destination


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        raise Error('alert webhook redirect refused')


def alert(config, subject, report):
    """Only a private config supplies destinations; never include it in errors."""
    content = json.dumps(report, ensure_ascii=False, sort_keys=True, indent=2)
    try:
        if config.get('transport') == 'smtp':
            if config.get('tls') not in ('ssl', 'starttls'):
                raise Error('SMTP alerts require TLS')
            message = EmailMessage()
            message['Subject'] = subject
            message['From'] = config['from']
            message['To'] = ', '.join(config['to'])
            message.set_content(content)
            factory = smtplib.SMTP_SSL if config['tls'] == 'ssl' else smtplib.SMTP
            kwargs = {'timeout': 15}
            if config['tls'] == 'ssl':
                kwargs['context'] = ssl.create_default_context()
            with factory(config['host'], int(config['port']), **kwargs) as smtp:
                if config['tls'] == 'starttls':
                    smtp.starttls(context=ssl.create_default_context())
                smtp.login(config['username'], config['password'])
                if smtp.send_message(message):
                    raise Error('alert recipients were rejected')
        elif config.get('transport') == 'webhook':
            if not str(config.get('url', '')).startswith('https://'):
                raise Error('alert webhook requires HTTPS')
            request = urllib.request.Request(config['url'], method='POST',
                headers={**config.get('headers', {}), 'Content-Type': 'application/json'},
                data=backup.json_bytes({'title': subject, 'report': report}))
            with urllib.request.build_opener(NoRedirect).open(request, timeout=15) as response:
                if not 200 <= response.status < 300:
                    raise Error('alert webhook did not accept the notification')
        else:
            raise Error('alert transport is not configured')
    except Error:
        raise
    except Exception:
        raise Error('alert delivery failed; inspect the private notification configuration') from None


def notification_due(previous, error, now, reminder):
    if previous:
        if (not isinstance(previous, dict) or not re.fullmatch(r'[a-f0-9]{64}', str(previous.get('signature', '')))
                or previous.get('error') is not None and not isinstance(previous.get('error'), str)):
            raise Error('notification receipt is invalid')
        age(previous.get('sent_at'), now)
    signature = hashlib.sha256((error or 'healthy').encode()).hexdigest()
    if signature != previous.get('signature'):
        return bool(error or previous.get('error')), signature
    return bool(error and age(previous.get('sent_at'), now) >= reminder), signature


def execute(args):
    with backup.locked(args.lock), backup.Processes() as processes:
        state = backup.private_directory(args.state_dir)
        now = dt.datetime.now(dt.timezone.utc)
        report = {'observed_at': now.isoformat(), 'healthy': False, 'error': None}
        failure_path = state / 'backup-failure.json'
        try:
            failure = backup_failure(failure_path, now)
            if failure:
                report['backup_failure'] = failure
                report['backup_service_error'] = failure.get('service_error',
                    'PITR full backup service failed; original details are unavailable')
            # Reserve time for mail even when a remote verification hangs.
            with backup.deadline(65):
                report['services'] = {}
                for unit in args.check_unit:
                    try:
                        report['services'][unit] = check_unit(unit, processes)
                    except ServiceFailure as exc:
                        if unit == FULL_BACKUP_UNIT:
                            failure = remember_backup_failure(failure_path, args.full_backup_status, now, failure, exc)
                            report['backup_failure'] = failure
                            report['backup_service_error'] = str(exc)
                            raise Error(BACKUP_PENDING) from None
                        raise
                status = object_json(backup.private_bytes(args.archive_status), 'archive status is invalid')
                remote = backup.CryptRemote(status['root'], args.rclone_config, processes)
                current = source_position(args, processes)
                report.update(source=current, frontier=status.get('last_complete_transaction_frontier'),
                              root=status['root'])
                report['lag_seconds'] = coverage(status, current, now, args.max_lag, args.max_stale)
                report['remote_receipt'] = verify_tail(remote, status)
                if shutil.disk_usage(args.disk_path).free < args.min_free_bytes:
                    raise Error('source filesystem free space is below the PITR warning threshold')
                latest = object_json(backup.private_bytes(args.full_backup_status), 'full backup receipt is invalid')
                report['full_backup'] = verify_full_backup(remote, status, latest, current, now, args.max_backup_age)
                if failure:
                    report['backup_failure'] = failure
                    newer_backup_completed(failure, latest, now)
                # Charge time spent reading remote data to the RPO measurement and
                # sample production again instead of reporting an old observation.
                now = dt.datetime.now(dt.timezone.utc)
                current = source_position(args, processes)
                report.update(source=current, observed_at=now.isoformat(),
                              lag_seconds=coverage(status, current, now, args.max_lag, args.max_stale))
                if failure:
                    clear_backup_failure(failure_path)
                    report['backup_failure_recovered_by'] = latest['run_id']
                report['healthy'] = True
        except (Error, OSError, ValueError, KeyError, TypeError) as exc:
            report['error'] = str(exc) if isinstance(exc, Error) else 'PITR monitoring input or source is unavailable'
        finally:
            # Reap stalled readers before sending mail, rather than allowing
            # their network timeouts to consume the notification budget.
            processes.close()
        now = dt.datetime.now(dt.timezone.utc)
        if args.test_alert:
            report['test_notification'] = True
        previous = {}
        notification_path = state / 'notification.json'
        try:
            if notification_path.exists():
                previous = object_json(backup.private_bytes(notification_path), 'notification receipt is invalid')
            due, signature = notification_due(previous, report['error'], now, args.reminder_seconds)
        except (Error, OSError, ValueError, TypeError):
            report['healthy'] = False
            report['error'] = report['error'] or 'notification receipt is invalid; previous delivery cannot be confirmed'
            previous = {}
            due, signature = notification_due(previous, report['error'], now, args.reminder_seconds)
        backup.atomic_json(state / 'health.json', report)
        if due or args.test_alert:
            try:
                config = object_json(backup.private_bytes(args.alert_config), 'alert configuration is invalid')
                subject = ('[New API PITR TEST]' if args.test_alert else
                           '[New API PITR FAILURE]' if report['error'] else '[New API PITR RECOVERED]')
                with backup.deadline(40):
                    alert(config, subject, report)
                if not args.test_alert:
                    backup.atomic_json(notification_path, {'signature': signature, 'error': report['error'],
                                       'sent_at': now.isoformat()})
                backup.atomic_json(state / 'last-delivery.json', {'status': 'accepted', 'at': now.isoformat(),
                                   'test': args.test_alert, 'signature': signature})
            except (Error, OSError, ValueError) as exc:
                backup.atomic_json(state / 'last-delivery.json', {'status': 'failed', 'at': now.isoformat(),
                                   'error': str(exc) if isinstance(exc, Error) else 'alert configuration is unavailable'})
                raise Error('PITR notification failed; notification state was not advanced') from None
        elif not previous:
            backup.atomic_json(notification_path, {'signature': signature, 'error': None, 'sent_at': now.isoformat()})
        return report


def parse_args(argv=None):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--container', default='newapi-mysql')
    parser.add_argument('--check-unit', action='append', default=[],
                        help='also alert on the latest result of this systemd service (repeatable)')
    parser.add_argument('--archive-status', default='/var/lib/newapi-binlog-archive/status.json')
    parser.add_argument('--full-backup-status', default='/var/lib/newapi-pitr-backup/latest-success.json')
    parser.add_argument('--rclone-config', default='/root/.config/rclone/rclone.conf')
    parser.add_argument('--alert-config', default='/root/.config/newapi-pitr/alerts.json')
    parser.add_argument('--state-dir', default='/var/lib/newapi-pitr-monitor')
    parser.add_argument('--lock', default='/run/newapi-pitr-monitor.lock')
    parser.add_argument('--disk-path', default='/opt/newapi')
    parser.add_argument('--min-free-bytes', type=int, default=5 * 1024**3)
    parser.add_argument('--max-lag', type=int, default=300)
    parser.add_argument('--max-stale', type=int, default=180)
    parser.add_argument('--max-backup-age', type=int, default=36 * 3600)
    parser.add_argument('--reminder-seconds', type=int, default=3600)
    parser.add_argument('--test-alert', action='store_true')
    args = parser.parse_args(argv)
    if min(args.min_free_bytes, args.max_lag, args.max_stale, args.max_backup_age, args.reminder_seconds) <= 0:
        parser.error('thresholds must be positive')
    if not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', args.container):
        parser.error('invalid container name')
    if any(not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_.@-]*\.service', unit) for unit in args.check_unit):
        parser.error('check-unit must be a service name without paths or options')
    return args


def main(argv=None):
    try:
        result = execute(parse_args(argv))
        print(json.dumps(result, sort_keys=True))
        return 0 if result['healthy'] else 1
    except (Error, OSError, ValueError):
        print('PITR monitor failed; inspect its private health and notification receipts', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
