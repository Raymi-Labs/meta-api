#!/usr/bin/env python3
"""Copy existing SMTP settings and the sole super-admin email into a private alert config."""
import argparse
import importlib.util
import json
import os
from pathlib import Path
import re
import sys

spec = importlib.util.spec_from_file_location('pitr_backup', Path(__file__).resolve().with_name('mysql-r2-backup.py'))
backup = importlib.util.module_from_spec(spec)
spec.loader.exec_module(backup)


def convert(options, recipients):
    required = ('SMTPServer', 'SMTPAccount', 'SMTPToken')
    if (not isinstance(options, dict) or not isinstance(recipients, list)
            or any(not isinstance(options.get(key), str) or not options[key] for key in required)
            or len(recipients) != 1):
        raise backup.BackupError('SMTP settings or a unique super-admin email are unavailable')
    sender = options.get('SMTPFrom') or options['SMTPAccount']
    if any(not isinstance(value, str) or not re.fullmatch(r'[^\s@<>,;]+@[^\s@<>,;]+', value) for value in [sender, *recipients]):
        raise backup.BackupError('SMTP sender or recipient address is invalid')
    if any(character.isspace() or character in '\x00/' for character in options['SMTPServer']):
        raise backup.BackupError('SMTP hostname is invalid')
    try:
        port = int(options.get('SMTPPort') or '587')
    except (ValueError, TypeError):
        raise backup.BackupError('SMTP port is invalid') from None
    if not 1 <= port <= 65535:
        raise backup.BackupError('SMTP port is invalid')
    return {'transport': 'smtp', 'host': options['SMTPServer'], 'port': port,
            'tls': 'ssl' if port == 465 or str(options.get('SMTPSSLEnabled')).lower() == 'true' else 'starttls',
            'username': options['SMTPAccount'], 'password': options['SMTPToken'],
            'from': sender, 'to': recipients,
            'source': 'existing New API SMTP and unique active super-admin email'}


def execute(args):
    backup.require_root()
    with backup.deadline(30), backup.Processes() as processes:
        query = ("SELECT JSON_OBJECTAGG(`key`,value) FROM `" + args.database + "`.options WHERE `key` IN "
                 "('SMTPServer','SMTPAccount','SMTPToken','SMTPFrom','SMTPPort','SMTPSSLEnabled'); "
                 "SELECT JSON_ARRAY(email) FROM `" + args.database + "`.users WHERE role=100 AND status=1 "
                 "AND deleted_at IS NULL AND email<>'' ORDER BY id;")
        raw = processes.capture(backup.mysql_command(args, 'mysql', ['--batch', '--raw', '--skip-column-names', '--execute=' + query]))
        rows = raw.decode('utf8').splitlines()
        config = convert(json.loads(rows[0]), [json.loads(row)[0] for row in rows[1:]])
        if args.write:
            destination = Path(args.output)
            backup.private_directory(str(destination.parent))
            backup.atomic_json(destination, config)
        return {'configured': True, 'transport': 'smtp', 'tls': config['tls'], 'recipients': 1, 'written': args.write}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--container', default='newapi-mysql')
    parser.add_argument('--database', default='new-api')
    parser.add_argument('--output', default='/root/.config/newapi-pitr/alerts.json')
    parser.add_argument('--write', action='store_true')
    args = parser.parse_args()
    if any(not re.fullmatch(r'[A-Za-z0-9][A-Za-z0-9_-]*', x) for x in [args.container, args.database]):
        parser.error('invalid source identifier')
    try:
        print(json.dumps(execute(args), sort_keys=True))
        return 0
    except (backup.BackupError, ValueError, OSError, IndexError, TypeError):
        print('PITR alert configuration failed; no credentials were printed', file=sys.stderr)
        return 1


if __name__ == '__main__':
    sys.exit(main())
