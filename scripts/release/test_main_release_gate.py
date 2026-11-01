import importlib.util
from pathlib import Path
import subprocess
import unittest
from unittest.mock import Mock

spec = importlib.util.spec_from_file_location('gate', Path(__file__).with_name('main-release-gate.py'))
gate = importlib.util.module_from_spec(spec)
spec.loader.exec_module(gate)


class MainReleaseGateTests(unittest.TestCase):
    def test_lightweight_and_annotated_tags_require_exact_commit(self):
        commit = 'a' * 40
        self.assertTrue(gate.exact_tag(commit + '\trefs/tags/v1.14.2\n', 'v1.14.2', commit))
        self.assertTrue(gate.exact_tag('b' * 40 + '\trefs/tags/v1.14.2\n' + commit + '\trefs/tags/v1.14.2^{}\n', 'v1.14.2', commit))
        self.assertFalse(gate.exact_tag('b' * 40 + '\trefs/tags/v1.14.2\n', 'v1.14.2', commit))
        self.assertFalse(gate.exact_tag(commit + '\trefs/tags/v1.14.20\n', 'v1.14.2', commit))

    def test_manual_or_ordinary_main_never_skips(self):
        lookup = Mock(side_effect=AssertionError('must not inspect release tags'))
        for event, version, old in [('workflow_dispatch', 'v1.14.2', 'v1.14.1'), ('push', 'v1.14.2', 'v1.14.2'), ('push', 'unsafe tag', 'v1.14.1')]:
            self.assertFalse(gate.should_defer(event, version, old, 'a' * 40, lookup))

    def test_tag_appears_within_budget_or_falls_back_on_timeout_and_network_failure(self):
        commit = 'a' * 40
        lookup = Mock(side_effect=['', commit + '\trefs/tags/v1.14.2\n'])
        self.assertTrue(gate.should_defer('push', 'v1.14.2', 'v1.14.1', commit, lookup, wait=Mock(), clock=Mock(side_effect=[0, 1])))
        for lookup in [Mock(return_value=''), Mock(side_effect=subprocess.TimeoutExpired('git', 10))]:
            self.assertFalse(gate.should_defer('push', 'v1.14.2', 'v1.14.1', commit, lookup, wait=Mock(), clock=Mock(side_effect=[0, 46])))


if __name__ == '__main__':
    unittest.main()
