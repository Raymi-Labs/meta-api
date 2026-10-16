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

def run():
    ROOT.mkdir(parents=True, exist_ok=True)
    path = ROOT / 'data.json'
    try:
        old = json.loads(path.read_text())
    except (OSError, ValueError):
        old = {}
    start = time.time()
    counters = read_counters()
    time.sleep(2)
    end = read_counters()
    elapsed = time.time() - start
    rates = {k: max(0, end[k] - counters[k]) * 8 / elapsed / 1e6 for k in ('rx_bytes', 'tx_bytes')}
    vn = subprocess.run(['vnstat', '--json', '-i', IFACE], capture_output=True, text=True, timeout=15)
    errors = []
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
    history = [x for x in old.get('history', []) if x.get('time', 0) > now - 86400][-1439:]
    history.append({'time': now, 'rx_mbps': rates['rx_bytes'], 'tx_mbps': rates['tx_bytes'], **latency})
    stamp = lambda value: datetime.fromtimestamp(value, timezone(timedelta(hours=8))).strftime('%Y-%m-%d %H:%M:%S')
    for point in history:
        point['label'] = stamp(point['time'])
    data = {'updated': now, 'started': next((i.get('created', {}).get('timestamp', now) for i in (traffic or {}).get('interfaces', []) if i.get('name') == IFACE), old.get('started', now)), 'interface': IFACE,
            'sample_seconds': round(elapsed, 2), 'rates': rates, 'counters': end,
            'uptime_seconds': float(Path('/proc/uptime').read_text().split()[0]),
            'traffic': traffic, 'latency': latency, 'history': history, 'errors': errors}
    data['updated_label'] = stamp(now)
    data['started_label'] = stamp(data['started'])
    temp = ROOT / 'data.json.tmp'
    temp.write_text(json.dumps(data, ensure_ascii=False))
    temp.chmod(0o644)
    temp.replace(path)

if __name__ == '__main__':
    run()
