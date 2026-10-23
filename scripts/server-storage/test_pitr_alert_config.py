import importlib.util
from pathlib import Path
import unittest

spec = importlib.util.spec_from_file_location('alert_config', Path(__file__).with_name('pitr-alert-config.py'))
m = importlib.util.module_from_spec(spec)
spec.loader.exec_module(m)

class AlertConfigTests(unittest.TestCase):
    def setUp(self):
        self.options = {'SMTPServer': 'smtp.test', 'SMTPAccount': 'ops@example.test', 'SMTPToken': 'private', 'SMTPPort': '587'}

    def test_tls_and_sole_recipient(self):
        value = m.convert(self.options, ['admin@example.test'])
        self.assertEqual(value['tls'], 'starttls')
        self.assertEqual(value['to'], ['admin@example.test'])
        self.assertEqual(value['from'], 'ops@example.test')
        self.assertEqual(m.convert({**self.options, 'SMTPPort': '465'}, ['admin@example.test'])['tls'], 'ssl')

    def test_no_guess_for_missing_or_multiple_recipients(self):
        for recipients in [[], ['a@b', 'c@d']]:
            with self.assertRaises(m.backup.BackupError):
                m.convert(self.options, recipients)

    def test_missing_secret_and_header_injection_fail(self):
        for options, recipients in [({**self.options, 'SMTPToken': ''}, ['a@b']), (self.options, ['a@b\nBcc:secret@elsewhere'])]:
            with self.assertRaises(m.backup.BackupError):
                m.convert(options, recipients)

    def test_invalid_shapes_ports_and_multiple_address_string_fail_closed(self):
        for options, recipients in [(None, ['a@b']), ([], ['a@b']),
                                    (self.options, 'admin@example.test'),
                                    (self.options, ['a@b,c@d']),
                                    ({**self.options, 'SMTPPort': 'private-invalid'}, ['a@b']),
                                    ({**self.options, 'SMTPServer': 'smtp.test\nprivate'}, ['a@b'])]:
            with self.subTest(options=type(options), recipients=type(recipients)), self.assertRaises(m.backup.BackupError) as error:
                m.convert(options, recipients)
            self.assertNotIn('private', str(error.exception))

if __name__ == '__main__':
    unittest.main()
