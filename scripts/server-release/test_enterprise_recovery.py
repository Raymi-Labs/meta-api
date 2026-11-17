"""The protected deployment entry must preserve receipt semantics on rollback."""
import os
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]

class EnterpriseRecoveryTests(unittest.TestCase):
    def test_incompatible_target_never_changes_compose(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);compose=root/'compose.yml';compose.write_text('original\n')
            code=r'''
source "$RELEASE_SCRIPT"
trap - EXIT
actual_image() { echo source; }
actual_image_id() { echo source-id; }
verify_enterprise_receipt_capability() { return 1; }
replace_compose_image() { echo mutated > "$COMPOSE_FILE"; }
TARGET_ID=incompatible
STATE_FILE="$APP_DIR/missing-state"
deploy_target "$COMPOSE_FILE"
'''
            result=subprocess.run(['bash','-c',code],env={**os.environ,'RELEASE_SCRIPT':str(ROOT/'scripts/newapi-release'),'NEWAPI_APP_DIR':directory,'NEWAPI_COMPOSE_FILE':str(compose)},text=True,capture_output=True,timeout=5)
            self.assertNotEqual(result.returncode,0)
            self.assertEqual(compose.read_text(),'original\n')
    def test_automatic_recovery_refuses_unsupported_receipts(self):
        code=r'''
source "$RELEASE_SCRIPT"
trap - EXIT
RECOVERY_REQUIRED=1
RECOVERY_ID=old
verify_enterprise_receipt_capability() { return 1; }
append_history() { return 0; }
docker() { echo UNEXPECTED_DOCKER; return 99; }
recover_previous_deployment
'''
        result=subprocess.run(['bash','-c',code],env={**os.environ,'RELEASE_SCRIPT':str(ROOT/'scripts/newapi-release')},text=True,capture_output=True,timeout=5)
        self.assertNotEqual(result.returncode,0)
        self.assertNotIn('UNEXPECTED_DOCKER',result.stdout)
        self.assertIn('automatic recovery refused',result.stderr)

    def test_first_upgrade_requires_recovery_before_switch(self):
        code = r'''
source "$RELEASE_SCRIPT"
trap - EXIT
actual_image_id() { echo sha256:old; }
mkdir() { :; }
run_preflight_pass() { :; }
schema_fingerprint() { echo stable; }
cleanup_preflight() { :; }
timeout() { shift 2; "$@"; }
docker() {
  case "$1" in
    exec)
      if [[ "$*" == *CAPABILITY_DB=newapi_preflight_* ]]; then
        echo 1
      fi ;;
    run)
      [[ "$*" == *sha256:old* ]] || return 98
      [[ "$SUPPORT" == yes ]] && echo enterprise_operations_v1 ;;
    rm) return 0 ;;
    *) return 99 ;;
  esac
}
preflight_target
printf 'READY_TO_SWITCH\n'
'''
        for support in ['yes', 'no']:
            with self.subTest(support=support):
                result = subprocess.run(['bash', '-c', code], env={**os.environ, 'RELEASE_SCRIPT': str(ROOT / 'scripts/newapi-release'), 'SUPPORT': support}, text=True, capture_output=True, timeout=5)
                self.assertEqual(result.returncode == 0, support == 'yes', result.stdout + result.stderr)
                self.assertEqual('READY_TO_SWITCH' in result.stdout, support == 'yes')
                if support == 'no':
                    self.assertIn('before switching', result.stderr)
