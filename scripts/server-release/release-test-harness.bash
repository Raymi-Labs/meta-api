# Isolated command doubles. Never source this file in a production shell.
source "$RELEASE_SCRIPT"

event() { printf '%s\n' "$*" >>"$APP_DIR/events"; }
actual_image() { cat "$APP_DIR/runtime"; }
actual_image_id() { actual_image; }
actual_version() { printf 'test-version\n'; }
image_id() { printf '%s\n' "$1"; }
image_version() { printf 'test-version\n'; }
resolve_target() {
  event resolve
  TARGET_IMAGE=new-image
  TARGET_ID=new-image
  TARGET_VERSION=test-new
}
create_database_backup() {
  event "backup:$(actual_image)"
  ACTIVE_BACKUP_DIR="$BACKUP_ROOT/test"
  mkdir -p "$ACTIVE_BACKUP_DIR"
  cp "$COMPOSE_FILE" "$ACTIVE_BACKUP_DIR/docker-compose.yml"
  if [[ "$CASE" == backup-failure ]]; then return 31; fi
}
preflight_target() {
  event "preflight:$(actual_image)"
  if [[ "$CASE" == preflight-failure ]]; then return 32; fi
}
cleanup_preflight() { event cleanup; }
curl() {
  if [[ "$CASE" == source-health-failure ]]; then return 22; fi
  if [[ "$CASE" == health-failure && "$(actual_image)" == new-image ]]; then return 22; fi
  return 0
}
docker() {
  local operation="$1" format file target
  shift
  case "$operation" in
    inspect)
      format="$2"
      case "$format" in
        '{{.State.Running}}') [[ -f "$APP_DIR/stopped" ]] && echo false || echo true ;;
        '{{.State.Status}}') [[ -f "$APP_DIR/stopped" ]] && echo exited || echo running ;;
        '{{.Image}}') actual_image ;;
        '{{.RestartCount}}') echo 0 ;;
        *) return 98 ;;
      esac ;;
    logs) return 0 ;;
    pull) event "pull:$1" ;;
    compose)
      [[ "$1" == -f ]] || return 98
      file="$2"
      shift 2
      if [[ "$1" == config ]]; then cat "$file"; return 0; fi
      [[ "$1" == up ]] || return 98
      target="$(awk '/image:/ {print $2; exit}' "$file")"
      event "up:$target"
      printf '%s\n' "$target" >"$APP_DIR/runtime"
      rm -f "$APP_DIR/stopped"
      if [[ "$target" == new-image ]]; then
        case "$CASE" in
          deploy-failure) touch "$APP_DIR/stopped"; return 33 ;;
          signal-HUP) kill -HUP "$$" ;;
          signal-INT) kill -INT "$$" ;;
          signal-TERM) kill -TERM "$$" ;;
        esac
      fi
      ;;
    *) return 98 ;;
  esac
}

if [[ "$CASE" == rollback-offline ]]; then
  command_rollback
else
  command_update "$@"
fi
