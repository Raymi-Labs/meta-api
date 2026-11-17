"""Exercise the real pre-switch guard without Docker or production access."""
import os
from pathlib import Path
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[2]

class EnterpriseCapabilityTests(unittest.TestCase):
    def check_guard(self, marker, support, expected):
        code = r'''
source "$RELEASE_SCRIPT"
trap - EXIT
log() { printf '%s\n' "$*"; }
timeout() { shift 2; "$@"; }
docker() {
  case "$1" in
    exec) printf '%s\n' "$MARKER" ;;
    run) test "$SUPPORT" = yes && printf 'enterprise_operations_v1\n' ;;
    rm) return 0 ;;
    *) return 99 ;;
  esac
}
verify_enterprise_receipt_capability sha256:fixture
'''
        result = subprocess.run(['bash','-c',code],env={**os.environ,'RELEASE_SCRIPT':str(ROOT/'scripts/newapi-release'),'MARKER':marker,'SUPPORT':support},capture_output=True,text=True,timeout=5)
        self.assertEqual(result.returncode==0,expected,result.stdout+result.stderr)
    def test_old_schema_needs_no_new_capability(self): self.check_guard('0','no',True)
    def test_new_schema_accepts_compatible_image(self): self.check_guard('1','yes',True)
    def test_incompatible_image_is_rejected(self): self.check_guard('1','no',False)
    def test_unknown_schema_fails_closed(self): self.check_guard('invalid','yes',False)
