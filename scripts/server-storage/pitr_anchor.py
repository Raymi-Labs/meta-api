"""Opt-in MySQL 8.0 snapshot coordination; no production actions at import time.

The SQL coordinate comes only from mysqldump's own consistent snapshot. A
container-local guard cancels the actual server connection, since terminating
the client alone does not reliably cancel a waiting FLUSH statement.
"""
import re
import select
import shlex
import subprocess
import threading
import time
import uuid


class AnchorError(Exception):
    pass


class Header:
    def __init__(self):
        self.buffer = b""
        self.anchor = None

    def feed(self, block):
        if self.anchor is not None:
            return
        self.buffer += block[:65537]
        end = self.buffer.find(b"\n-- Current Database:")
        if end < 0:
            if len(self.buffer) > 65536:
                raise AnchorError("snapshot header is missing or too large")
            return
        if end > 65536:
            raise AnchorError("snapshot header is too large")
        matches = re.findall(rb"^-- CHANGE MASTER TO MASTER_LOG_FILE='([A-Za-z0-9_.-]+)', MASTER_LOG_POS=([0-9]+);\r?$",
                             self.buffer[:end], re.M)
        if len(matches) != 1 or int(matches[0][1]) < 4:
            raise AnchorError("snapshot must contain exactly one valid binlog coordinate")
        filename = matches[0][0].decode("ascii")
        if not re.fullmatch(r"[A-Za-z0-9_-][A-Za-z0-9_.-]*\.[0-9]+", filename):
            raise AnchorError("snapshot binlog filename is invalid")
        self.anchor = {"file": filename, "position": int(matches[0][1])}
        self.buffer = b""

    def finish(self):
        if self.anchor is None:
            raise AnchorError("snapshot binlog coordinate was not found")
        return self.anchor.copy()


def mysql_shell():
    return 'MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" mysql --protocol=SOCKET --skip-reconnect --user=root --batch --skip-column-names --unbuffered'


def query(container, sql, timeout=5):
    try:
        result = subprocess.run(["docker", "exec", container, "sh", "-c",
                                 mysql_shell() + " --execute=" + shlex.quote(sql)],
                                stdin=subprocess.DEVNULL, stdout=subprocess.PIPE,
                                stderr=subprocess.DEVNULL, timeout=timeout, check=True)
    except (OSError, subprocess.SubprocessError):
        raise AnchorError("snapshot coordination query failed") from None
    return result.stdout.decode("ascii").strip()


class InstanceLock:
    """Hold the DDL lock with an idle lease, so an orphan expires in 10 seconds."""
    def __init__(self, container):
        self.container = container
        self.child = None
        self.connection_id = None
        self.connection_pid = None
        self.stop = threading.Event()
        self.failed = False
        self.thread = None
        self.closed = False
        self.lease_token = uuid.uuid4().hex

    def line(self, timeout):
        if not select.select([self.child.stdout], [], [], timeout)[0]:
            raise AnchorError("snapshot instance lock response timed out")
        line = self.child.stdout.readline(128)
        if not line.endswith(b"\n"):
            raise AnchorError("snapshot instance lock connection closed")
        return line.strip()

    def send(self, sql):
        self.child.stdin.write((sql + "\n").encode("ascii"))
        self.child.stdin.flush()

    def acquire(self):
        try:
            self.child = subprocess.Popen(["docker", "exec", "-i", self.container,
                                           "sh", "-c", mysql_shell().replace(" mysql ", " exec mysql ")],
                                          stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                          stderr=subprocess.DEVNULL, bufsize=0)
            self.send("SET SESSION wait_timeout=10; SET SESSION lock_wait_timeout=5; SET @newapi_pitr_lease='"
                      + self.lease_token + "'; SELECT CONNECTION_ID();")
            connection = self.line(5)
            if not re.fullmatch(rb"[1-9][0-9]*", connection):
                raise AnchorError("snapshot instance lock identity is invalid")
            self.connection_id = int(connection)
            self.send("SELECT a.ATTR_VALUE,b.ATTR_VALUE FROM performance_schema.session_connect_attrs a "
                      "JOIN performance_schema.session_connect_attrs b ON a.PROCESSLIST_ID=b.PROCESSLIST_ID "
                      "WHERE a.PROCESSLIST_ID=CONNECTION_ID() AND a.ATTR_NAME='_pid' AND b.ATTR_NAME='program_name';")
            identity = self.line(5).split(b"\t")
            if len(identity) != 2 or not re.fullmatch(rb"[1-9][0-9]*", identity[0]) or identity[1] != b"mysql":
                raise AnchorError("snapshot instance lock client identity is invalid")
            self.connection_pid = int(identity[0])
            self.send("LOCK INSTANCE FOR BACKUP; SELECT 'NEWAPI_PITR_LOCKED';")
            if self.line(6) != b"NEWAPI_PITR_LOCKED":
                raise AnchorError("snapshot instance lock was not acquired")
            self.thread = threading.Thread(target=self.heartbeat, daemon=True)
            self.thread.start()
            return self
        except (OSError, AnchorError):
            self.close()
            raise AnchorError("snapshot instance lock could not be acquired") from None

    def heartbeat(self):
        while not self.stop.wait(2):
            try:
                self.send("SELECT @newapi_pitr_lease;")
                if self.line(3) != self.lease_token.encode("ascii"):
                    raise AnchorError("snapshot instance lock heartbeat failed")
            except (OSError, AnchorError):
                self.failed = True
                return

    def check(self):
        if self.failed or self.child is None or self.child.poll() is not None:
            raise AnchorError("snapshot instance lock was lost")

    def close(self):
        if self.closed:
            return
        self.stop.set()
        if self.thread:
            self.thread.join(timeout=4)
        confirmed = self.connection_id is None
        if self.connection_id is not None:
            identity = str(self.connection_id)
            try:
                # Bind the ID to a random variable in this exact lease session.
                # Check and conditional KILL use one non-reconnecting control
                # connection, so a server restart cannot reassign the ID between
                # two separately established administrative connections.
                identity_query = ("SELECT COUNT(*) FROM performance_schema.threads t JOIN "
                                  "performance_schema.user_variables_by_thread v ON v.THREAD_ID=t.THREAD_ID "
                                  "WHERE t.PROCESSLIST_ID=" + identity + " AND t.PROCESSLIST_USER='root' "
                                  "AND t.PROCESSLIST_HOST='localhost' AND v.VARIABLE_NAME='newapi_pitr_lease' "
                                  "AND CAST(v.VARIABLE_VALUE AS CHAR)='" + self.lease_token + "'")
                if self.connection_pid is not None:
                    identity_query += (" AND EXISTS(SELECT 1 FROM performance_schema.session_connect_attrs a "
                                       "WHERE a.PROCESSLIST_ID=t.PROCESSLIST_ID AND a.ATTR_NAME='_pid' "
                                       "AND a.ATTR_VALUE='" + str(self.connection_pid) + "') "
                                       "AND EXISTS(SELECT 1 FROM performance_schema.session_connect_attrs a "
                                       "WHERE a.PROCESSLIST_ID=t.PROCESSLIST_ID AND a.ATTR_NAME='program_name' AND a.ATTR_VALUE='mysql')")
                command = ("SET @newapi_kill=IF((" + identity_query + ")=1,'KILL CONNECTION " + identity + "','DO 0'); "
                           "PREPARE newapi_cancel FROM @newapi_kill; EXECUTE newapi_cancel; DEALLOCATE PREPARE newapi_cancel; "
                           + identity_query)
                limit = time.monotonic() + 3
                while time.monotonic() < limit:
                    confirmed = query(self.container, command, timeout=max(.1, min(2, limit-time.monotonic()))) == "0"
                    if confirmed:
                        break
                    # KILL marks the server thread; removal of its P_S row can
                    # lag the KILL response briefly while the thread unwinds.
                    time.sleep(.05)
            except AnchorError:
                confirmed = False
        if self.child:
            for stream in (self.child.stdin, self.child.stdout):
                try:
                    stream.close()
                except OSError:
                    pass
            try:
                self.child.wait(timeout=2)
            except subprocess.TimeoutExpired:
                self.child.kill()
                self.child.wait()
        if not confirmed:
            raise AnchorError("instance lock cleanup could not be confirmed; idle lease is limited to 10 seconds")
        self.closed = True


# This is executed inside the MySQL container. No credentials enter argv or
# receipts. PID and program_name together identify only this dump connection.
GUARD_FUNCTIONS = r'''
# CLOCK_BOOTTIME from procfs is monotonic, unlike date during an NTP step.
# procfs reports centiseconds; add one tick when reporting an upper bound.
clock_now() {
  local value rest whole fraction
  read -r value rest < /proc/uptime || return 1
  [[ "$value" =~ ^[0-9]+\.[0-9]{2}$ ]] || return 1
  whole=${value%%.*}; fraction=${value#*.}
  clock_ns=$(( (10#$whole*100 + 10#$fraction) * 10000000 ))
}
sql() {
  local limit=2s kill_after=1s remaining_ms
  if [ -n "${cancel_deadline_ns:-}" ]; then
    clock_now || return 1
    remaining_ms=$(( (cancel_deadline_ns-clock_ns)/1000000 ))
    [ "$remaining_ms" -gt 0 ] || return 124
    [ "$remaining_ms" -le 2000 ] || remaining_ms=2000
    printf -v limit '%d.%03ds' "$((remaining_ms/1000))" "$((remaining_ms%1000))"
    kill_after=0.2s
  fi
  MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" timeout --signal=TERM --kill-after="$kill_after" "$limit" mysql --protocol=SOCKET --skip-reconnect --user=root --batch --skip-column-names --execute="$1" 2>/dev/null
}
terminate_client() {
  # Signals are bound to this exact process lifetime, even when P_S is unable
  # to answer. A vanished client still requires server-side confirmation.
  [ -r "/proc/$dump_pid/stat" ] || return 0
  [ -n "${dump_start:-}" ] || return 1
  if [ "$(awk '{print $22}' "/proc/$dump_pid/stat" 2>/dev/null)" != "$dump_start" ]; then
    [ ! -d "/proc/$dump_pid" ] && return 0
    return 1
  fi
  if [ "$(cat "/proc/$dump_pid/comm" 2>/dev/null)" != mysqldump ]; then
    [ ! -d "/proc/$dump_pid" ] && return 0
    return 1
  fi
  kill -TERM "$dump_pid" 2>/dev/null || :
}
connections() {
  if [ -r "/proc/$dump_pid/stat" ] && [ -n "${dump_start:-}" ]; then
    [ "$(awk '{print $22}' "/proc/$dump_pid/stat")" = "$dump_start" ] || return 1
  fi
  sql "SELECT a.PROCESSLIST_ID FROM performance_schema.session_connect_attrs a JOIN performance_schema.session_connect_attrs b ON a.PROCESSLIST_ID=b.PROCESSLIST_ID JOIN performance_schema.threads t ON t.PROCESSLIST_ID=a.PROCESSLIST_ID WHERE a.ATTR_NAME='_pid' AND a.ATTR_VALUE='$dump_pid' AND b.ATTR_NAME='program_name' AND b.ATTR_VALUE='mysqldump' AND t.PROCESSLIST_USER='root' AND t.PROCESSLIST_HOST='localhost'"
}
kill_connection() {
  # Revalidate in the same non-reconnecting control session as KILL. A server
  # restart after the earlier inventory must not turn a reused ID into a target.
  sql "SET @newapi_kill=IF(EXISTS(SELECT 1 FROM performance_schema.session_connect_attrs a JOIN performance_schema.session_connect_attrs b ON a.PROCESSLIST_ID=b.PROCESSLIST_ID JOIN performance_schema.threads t ON t.PROCESSLIST_ID=a.PROCESSLIST_ID WHERE a.PROCESSLIST_ID=$1 AND a.ATTR_NAME='_pid' AND a.ATTR_VALUE='$dump_pid' AND b.ATTR_NAME='program_name' AND b.ATTR_VALUE='mysqldump' AND t.PROCESSLIST_USER='root' AND t.PROCESSLIST_HOST='localhost'),'KILL CONNECTION $1','DO 0'); PREPARE newapi_cancel FROM @newapi_kill; EXECUTE newapi_cancel; DEALLOCATE PREPARE newapi_cancel" >/dev/null
}
unlock_evidence() {
  local saw_transaction=0
  [ ! -f "$run_dir/transaction-starting" ] || saw_transaction=1
  if [ -r "/proc/$dump_pid/stat" ] && [ -n "${dump_start:-}" ]; then
    [ "$(awk '{print $22}' "/proc/$dump_pid/stat")" = "$dump_start" ] || return 1
  fi
  # Absence of a metadata lock is never sufficient: an identified live dump
  # must also have completed UNLOCK, or have an ACTIVE snapshot transaction
  # after mysqldump's post-FTWRL Starting-transaction marker. History is small,
  # so the transaction evidence remains usable after early statements age out.
  sql "SELECT CASE WHEN EXISTS(SELECT 1 FROM performance_schema.events_statements_history h WHERE h.THREAD_ID=t.THREAD_ID AND h.SQL_TEXT='UNLOCK TABLES' AND h.MYSQL_ERRNO=0 AND h.END_EVENT_ID IS NOT NULL) THEN 'completed-unlock' WHEN $saw_transaction=1 AND EXISTS(SELECT 1 FROM performance_schema.events_transactions_current x WHERE x.THREAD_ID=t.THREAD_ID AND x.STATE='ACTIVE' AND x.END_EVENT_ID IS NULL AND x.ISOLATION_LEVEL='REPEATABLE READ' AND x.AUTOCOMMIT='NO') THEN 'active-snapshot' ELSE 'pending' END FROM performance_schema.session_connect_attrs a JOIN performance_schema.session_connect_attrs b ON a.PROCESSLIST_ID=b.PROCESSLIST_ID JOIN performance_schema.threads t ON t.PROCESSLIST_ID=a.PROCESSLIST_ID WHERE a.ATTR_NAME='_pid' AND a.ATTR_VALUE='$dump_pid' AND b.ATTR_NAME='program_name' AND b.ATTR_VALUE='mysqldump' AND t.PROCESSLIST_USER='root' AND t.PROCESSLIST_HOST='localhost' AND NOT EXISTS(SELECT 1 FROM performance_schema.metadata_locks m WHERE m.OWNER_THREAD_ID=t.THREAD_ID AND m.OBJECT_TYPE IN ('GLOBAL','COMMIT'))"
}
cancel_sql() {
  local ids id remaining
  # The bound lives inside the container: killing a host docker-exec client
  # does not stop the SQL cancellation command it has already launched.
  clock_now || { terminate_client; return 1; }
  cancel_deadline_ns=$((clock_ns+8000000000))
  ids=$(connections) || ids=
  for id in $ids; do
    case "$id" in *[!0-9]*|'') return 1;; esac
    kill_connection "$id" || :
  done
  # Always attempt the identity-checked signal, including when the first
  # inventory failed. This can release socket backpressure before rechecking.
  terminate_client || return 1
  while clock_now && [ "$clock_ns" -lt "$cancel_deadline_ns" ]; do
    if ! remaining=$(connections); then sleep 0.05; continue; fi
    if [ -z "$remaining" ]; then return 0; fi
    for id in $remaining; do
      case "$id" in *[!0-9]*|'') return 1;; esac
      kill_connection "$id" || :
    done
    sleep 0.05
  done
  return 1
}
publish_ready() {
  # The fast-dump completion path and watchdog may observe proof together.
  # Claim one publisher so the timestamp and evidence method remain paired.
  if mkdir "$run_dir/ready-publishing" 2>/dev/null; then
    printf '%s\n' "$2" > "$run_dir/ready-method"
    printf '%s\n' "$1" > "$run_dir/ready.tmp"
    mv "$run_dir/ready.tmp" "$run_dir/ready"
  fi
}
abort_dump() {
  : > "$run_dir/timed-out"
  if cancel_sql; then : > "$run_dir/cancel-confirmed"; else : > "$run_dir/cancel-failed"; fi
}
'''


def dump_script(run_id, options, maximum_seconds, startup_seconds=5):
    if (not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", run_id)
            or not isinstance(maximum_seconds, int) or maximum_seconds <= 0
            or not isinstance(startup_seconds, int) or not 1 <= startup_seconds <= 5):
        raise AnchorError("snapshot guard parameters are invalid")
    directory = "/tmp/newapi-r2-backup-" + run_id
    return ("NEWAPI_BACKUP_RUN=" + shlex.quote(run_id) + "\nset -u\numask 077\nrun_dir="
            + shlex.quote(directory) + "\n[ -d \"$run_dir\" ] && [ ! -L \"$run_dir\" ] || exit 70\n"
            + GUARD_FUNCTIONS + r'''
dump_pid=; guard_pid=; monitor_pid=
cleanup() {
  trap '' TERM INT
  if [ -n "$dump_pid" ] && [ ! -f "$run_dir/done" ]; then
    if cancel_sql; then : > "$run_dir/cancel-confirmed"; else : > "$run_dir/cancel-failed"; fi
  fi
  [ -z "$guard_pid" ] || kill "$guard_pid" 2>/dev/null || :
  [ -z "$monitor_pid" ] || kill "$monitor_pid" 2>/dev/null || :
}
trap cleanup EXIT
trap 'exit 143' TERM INT
mkfifo -m 600 "$run_dir/events" || exit 71
clock_now || exit 72
started=$clock_ns
(
  while IFS= read -r line; do
    if [ "$line" = '-- Starting transaction...' ]; then
      clock_now || exit 72
      printf '%s\n' "$((clock_ns-started+10000000))" > "$run_dir/transaction-starting"
    elif [ "$line" = '-- Setting savepoint...' ] && [ ! -f "$run_dir/post-unlock-marker" ]; then
      clock_now || exit 72
      printf '%s\n' "$((clock_ns-started+10000000))" > "$run_dir/post-unlock-marker.tmp"
      mv "$run_dir/post-unlock-marker.tmp" "$run_dir/post-unlock-marker"
    fi
  done < "$run_dir/events"
) &
monitor_pid=$!
MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" mysqldump --protocol=SOCKET --user=root --verbose '''
            + " ".join(shlex.quote(value) for value in options) + r''' 2> "$run_dir/events" &
dump_pid=$!
dump_start=$(awk '{print $22}' "/proc/$dump_pid/stat" 2>/dev/null || :)
printf '%s %s %s\n' "$$" "$dump_pid" "$dump_start" > "$run_dir/pids"
(
  while [ ! -f "$run_dir/done" ]; do
    clock_now || { abort_dump; exit 72; }
    elapsed=$((clock_ns-started+10000000))
    [ -f "$run_dir/guard-first-poll" ] || printf '%s\n' "$elapsed" > "$run_dir/guard-first-poll"
    if [ ! -f "$run_dir/ready" ] && [ "$elapsed" -lt ''' + str(startup_seconds * 1000000000) + r''' ]; then
      evidence=pending
      if [ -f "$run_dir/transaction-starting" ]; then
        evidence=$(unlock_evidence) || evidence=unobservable
      fi
      clock_now || { abort_dump; exit 72; }
      elapsed=$((clock_ns-started+10000000))
      if { [ "$evidence" = completed-unlock ] || [ "$evidence" = active-snapshot ]; } && [ "$elapsed" -le ''' + str(startup_seconds * 1000000000) + r''' ]; then
        publish_ready "$elapsed" "$evidence"
      elif [ -f "$run_dir/post-unlock-marker" ] && [ "$(cat "$run_dir/post-unlock-marker")" -le ''' + str(startup_seconds * 1000000000) + r''' ]; then
        # A tiny completed dump may disconnect before P_S can be sampled. Its
        # post-UNLOCK marker is independently sufficient only if observed on time.
        publish_ready "$(cat "$run_dir/post-unlock-marker")" post-unlock-marker
      fi
    fi
    late_ready=0
    if [ -f "$run_dir/ready" ] && [ "$(cat "$run_dir/ready")" -gt ''' + str(startup_seconds * 1000000000) + r''' ]; then late_ready=1; fi
    if { [ ! -f "$run_dir/ready" ] && [ "$elapsed" -ge ''' + str(startup_seconds * 1000000000) + r''' ]; } || [ "$late_ready" -eq 1 ] || [ "$elapsed" -ge ''' + str(maximum_seconds * 1000000000) + r''' ]; then
      printf '%s\n' "$elapsed" > "$run_dir/cancellation-started"
      abort_dump
      exit
    fi
    sleep 0.05
  done
) &
guard_pid=$!
printf '%s\n' "$guard_pid" > "$run_dir/guard-pid"
[ ! -e "$run_dir/cancelled" ] || exit 143
wait "$dump_pid"
result=$?
wait "$monitor_pid" || result=74
if [ "$result" -eq 0 ] && [ ! -f "$run_dir/ready" ] && [ -f "$run_dir/post-unlock-marker" ] && [ "$(cat "$run_dir/post-unlock-marker")" -le ''' + str(startup_seconds * 1000000000) + r''' ]; then
  publish_ready "$(cat "$run_dir/post-unlock-marker")" post-unlock-marker
fi
if [ "$result" -eq 0 ]; then : > "$run_dir/done"; fi
if [ -f "$run_dir/timed-out" ] || [ ! -f "$run_dir/ready" ] || [ -f "$run_dir/cancel-failed" ]; then result=75; fi
if [ -f "$run_dir/ready" ] && [ "$(cat "$run_dir/ready")" -gt ''' + str(startup_seconds * 1000000000) + r''' ]; then result=76; fi
exit "$result"
''')


def cancel_script(run_id):
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,128}", run_id):
        raise AnchorError("snapshot run identity is invalid")
    return ("run_dir=" + shlex.quote("/tmp/newapi-r2-backup-" + run_id) + "\n"
            + GUARD_FUNCTIONS + r'''
[ -d "$run_dir" ] && [ ! -L "$run_dir" ] || exit 0
: > "$run_dir/cancelled"
[ -r "$run_dir/pids" ] || exit 0
read -r wrapper dump_pid dump_start < "$run_dir/pids" || exit 71
case "$wrapper:$dump_pid:$dump_start" in *[!0-9:]*|:*) exit 72;; esac
if [ -r "/proc/$wrapper/cmdline" ]; then
  case "$(tr '\0' ' ' < "/proc/$wrapper/cmdline")" in
    *NEWAPI_BACKUP_RUN=''' + run_id + r'''*) ;;
    *) exit 73;;
  esac
fi
cancel_sql || exit 74
kill -TERM "$wrapper" 2>/dev/null || :
''')
