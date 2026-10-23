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
                confirmed = query(self.container, command) == "0"
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
sql() { MYSQL_PWD="${MYSQL_ROOT_PASSWORD:?}" timeout --signal=TERM --kill-after=1s 2s mysql --protocol=SOCKET --skip-reconnect --user=root --batch --skip-column-names --execute="$1" 2>/dev/null; }
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
cancel_sql() {
  local ids id remaining
  ids=$(connections) || return 1
  for id in $ids; do
    case "$id" in *[!0-9]*|'') return 1;; esac
    kill_connection "$id" || :
  done
  # Kill the client after cancelling its server connection, then recheck to
  # close the race where a connection was still being established above.
  kill -TERM "$dump_pid" 2>/dev/null || :
  for attempt in {1..20}; do
    remaining=$(connections) || return 1
    if [ -z "$remaining" ]; then return 0; fi
    for id in $remaining; do kill_connection "$id" || :; done
    sleep 0.05
  done
  return 1
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
started=$(date +%s%N)
case "$started" in *[!0-9]*|'') exit 72;; esac
(
  while IFS= read -r line; do
    if [ "$line" = '-- Setting savepoint...' ] && [ ! -f "$run_dir/ready" ]; then
      now=$(date +%s%N)
      printf '%s\n' "$((now-started))" > "$run_dir/ready.tmp"
      mv "$run_dir/ready.tmp" "$run_dir/ready"
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
    now=$(date +%s%N)
    elapsed=$((now-started))
    if { [ ! -f "$run_dir/ready" ] && [ "$elapsed" -ge ''' + str(startup_seconds * 1000000000) + r''' ]; } || [ "$elapsed" -ge ''' + str(maximum_seconds * 1000000000) + r''' ]; then
      : > "$run_dir/timed-out"
      if cancel_sql; then : > "$run_dir/cancel-confirmed"; else : > "$run_dir/cancel-failed"; fi
      exit
    fi
    sleep 0.05
  done
) &
guard_pid=$!
[ ! -e "$run_dir/cancelled" ] || exit 143
wait "$dump_pid"
result=$?
if [ "$result" -eq 0 ]; then : > "$run_dir/done"; fi
wait "$monitor_pid" || result=74
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
