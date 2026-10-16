#!/usr/bin/env python3
"""Write an atomic, bounded monitoring snapshot; no credentials or request logs."""
import json, os, re, subprocess, time
from pathlib import Path
from datetime import datetime, timezone, timedelta

ROOT = Path(os.environ.get('MONITOR_ROOT', '/var/lib/relay-monitor'))
IFACE = os.environ.get('MONITOR_INTERFACE', 'ens3')
TARGET = os.environ.get('MONITOR_TARGET', '104.168.114.150')

def read_counters():
    return {name: int(Path(f'/sys/class/net/{IFACE}/statistics/{name}').read_text())
            for name in ('rx_bytes', 'tx_bytes', 'rx_dropped', 'tx_dropped', 'rx_errors', 'tx_errors')}

def read_cpu():
    # guest/guest_nice are already included in user/nice; count only first 8.
    return [int(v) for v in Path('/proc/stat').read_text().splitlines()[0].split()[1:9]]

def cpu_usage(before, after):
    delta = [b - a for a, b in zip(before, after)]
    if len(delta) != 8 or any(v < 0 for v in delta) or sum(delta) <= 0:
        return None
    return round(100 * (sum(delta) - delta[3] - delta[4]) / sum(delta), 2)

def memory_usage(text):
    values = {line.split(':')[0]: int(line.split()[1]) * 1024
              for line in text.splitlines() if ':' in line}
    total = values['MemTotal']
    available = values['MemAvailable']
    used = max(0, total - available)
    return {'total': total, 'used': used, 'available': available,
            'percent': round(100 * used / total, 2) if total else None,
            'swap_total': values.get('SwapTotal', 0),
            'swap_used': max(0, values.get('SwapTotal', 0) - values.get('SwapFree', 0))}

def server_status(cpu):
    disk = os.statvfs('/')
    total = disk.f_blocks * disk.f_frsize
    used = (disk.f_blocks - disk.f_bfree) * disk.f_frsize
    available = disk.f_bavail * disk.f_frsize
    return {'cpu_percent': cpu, 'cpu_cores': os.cpu_count(),
            'memory': memory_usage(Path('/proc/meminfo').read_text()),
            'disk': {'total': total, 'used': used, 'available': available,
                     'percent': round(100 * used / (used + available), 2) if used + available else None},
            'load': list(os.getloadavg()),
            'uptime_seconds': int(float(Path('/proc/uptime').read_text().split()[0]))}

def rolling_usage(rows, created, updated, window, bucket):
    """Prorate boundary buckets; incomplete days remain usable."""
    rows = [r for r in rows if r['timestamp'] < updated]
    start = max(created, updated - window, min((r['timestamp'] for r in rows), default=updated))
    measured = {'rx': 0.0, 'tx': 0.0}
    for row in rows:
        left = max(created, row['timestamp'])
        right = min(updated, row['timestamp'] + bucket)
        overlap = max(0, right - max(start, left))
        if right > left and overlap:
            for key in measured:
                measured[key] += row[key] * overlap / (right - left)
    return {k: round(v) for k, v in measured.items()}, max(0, updated - start)

def month_remaining(updated):
    current = datetime.fromtimestamp(updated, timezone(timedelta(hours=8)))
    end = current.replace(year=current.year + (current.month == 12),
                          month=1 if current.month == 12 else current.month + 1,
                          day=1, hour=0, minute=0, second=0, microsecond=0)
    return max(0, int(end.timestamp()) - updated)

def summarize(interface):
    """Keep the 24h card; forecast month end using a rolling 168h mean."""
    if not interface:
        return None
    traffic = interface.get('traffic', {})
    created = interface.get('created', {}).get('timestamp', 0)
    updated = interface.get('updated', {}).get('timestamp', 0)
    measured, seconds = rolling_usage(traffic.get('fiveminute', []), created, updated, 86400, 300)
    forecast_rows = traffic.get('hour', [])
    # Transitional fallback before the daemon persists its first hourly bucket.
    forecast_measured, forecast_seconds = rolling_usage(
        forecast_rows or traffic.get('fiveminute', []), created, updated,
        7 * 86400, 3600 if forecast_rows else 300)
    projection = {k: round(forecast_measured[k] * 30 * 86400 / forecast_seconds)
                  for k in ('rx', 'tx')} if forecast_seconds else None
    date = interface.get('updated', {}).get('date', {})
    today = next((r for r in traffic.get('day', []) if r['date'] == date), None)
    month = next((r for r in traffic.get('month', [])
                  if r['date'].get('year') == date.get('year') and r['date'].get('month') == date.get('month')), None)
    remaining = month_remaining(updated)
    month_projection = {k: round(month[k] + forecast_measured[k] * remaining / forecast_seconds)
                        for k in ('rx', 'tx')} if month and forecast_seconds else None
    pair = lambda r: {k: r[k] for k in ('rx', 'tx')} if r else None
    return {'today': pair(today), 'month': pair(month), 'total': pair(traffic.get('total')),
            'recent': measured if seconds else None, 'observed_seconds': seconds,
            'forecast_30d': projection, 'forecast_month': month_projection,
            'forecast_observed_seconds': forecast_seconds, 'forecast_recent': forecast_measured,
            'month_remaining_seconds': remaining, 'as_of': updated}

def run():
    ROOT.mkdir(parents=True, exist_ok=True)
    path = ROOT / 'data.json'
    try:
        old = json.loads(path.read_text())
    except (OSError, ValueError):
        old = {}
    errors = []
    try:
        cpu_before = read_cpu()
    except (OSError, ValueError):
        cpu_before = None
    start = time.monotonic()
    counters = read_counters()
    time.sleep(2)
    end = read_counters()
    elapsed = time.monotonic() - start
    try:
        server = server_status(cpu_usage(cpu_before, read_cpu()) if cpu_before else None)
        if server["cpu_percent"] is None:
            errors.append("CPU 采样暂不可用")
    except (OSError, ValueError, KeyError):
        server = None
        errors.append("服务器状态读取失败")
    rates = {k: max(0, end[k] - counters[k]) * 8 / elapsed / 1e6 for k in ('rx_bytes', 'tx_bytes')}
    vn = subprocess.run(['vnstat', '--json', '-i', IFACE], capture_output=True, text=True, timeout=15)
    try:
        traffic = json.loads(vn.stdout) if vn.returncode == 0 else None
    except ValueError:
        traffic = None
    if traffic is None:
        errors.append('vnStat 数据读取失败')
    ping = subprocess.run(['ping', '-n', '-c', '5', '-i', '0.2', '-W', '2', TARGET], capture_output=True, text=True, timeout=15)
    loss = re.search(r'([\d.]+)% packet loss', ping.stdout)
    rtt = re.search(r'= ([\d.]+)/([\d.]+)/([\d.]+)/([\d.]+) ms', ping.stdout)
    latency = {'target': TARGET, 'loss': float(loss[1]) if loss else None,
               'min': float(rtt[1]) if rtt else None, 'avg': float(rtt[2]) if rtt else None,
               'max': float(rtt[3]) if rtt else None, 'mdev': float(rtt[4]) if rtt else None}
    if not rtt:
        errors.append('业务服务器 ICMP 探测无有效延迟')
    now = int(time.time())
    history = [{k: x[k] for k in ('time', 'rx_mbps', 'tx_mbps')}
               for x in old.get('history', []) if now - 86400 < x.get('time', 0) <= now][-1439:]
    history.append({'time': now, 'rx_mbps': rates['rx_bytes'], 'tx_mbps': rates['tx_bytes']})
    stamp = lambda value: datetime.fromtimestamp(value, timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
    interface = next((i for i in (traffic or {}).get('interfaces', []) if i.get('name') == IFACE), None)
    summary = summarize(interface)
    if summary and now - summary['as_of'] > 180:
        errors.append('流量汇总超过 3 分钟未更新，月预估可能过期')
    data = {'updated': now, 'started': next((i.get('created', {}).get('timestamp', now) for i in (traffic or {}).get('interfaces', []) if i.get('name') == IFACE), old.get('started', now)), 'interface': IFACE,
            'sample_seconds': round(elapsed, 2), 'rates': rates,
            'server': server, 'summary': summary, 'latency': latency, 'history': history, 'errors': errors,
            'history_range': [stamp(history[0]['time']), stamp(history[-1]['time'])]}
    data['updated_label'] = stamp(now)
    data['started_label'] = stamp(data['started'])
    temp = ROOT / 'data.json.tmp'
    temp.write_text(json.dumps(data, ensure_ascii=False))
    temp.chmod(0o644)
    temp.replace(path)

if __name__ == '__main__':
    run()
