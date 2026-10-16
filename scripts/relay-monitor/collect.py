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

def summarize(interface):
    """Project a 30-day equivalent from measured bytes, never from sampled peaks."""
    if not interface:
        return None
    traffic = interface.get('traffic', {})
    created = interface.get('created', {}).get('timestamp', 0)
    updated = interface.get('updated', {}).get('timestamp', 0)
    cutoff = max(created, updated - 86400)
    # Do not charge a whole old bucket to a partially overlapping window.
    rows = [r for r in traffic.get('fiveminute', [])
            if r['timestamp'] < updated and
            (r['timestamp'] >= cutoff or
             (cutoff == created and r['timestamp'] <= created < r['timestamp'] + 300))]
    start = max(cutoff, min((r['timestamp'] for r in rows), default=updated))
    seconds = max(0, updated - start)
    measured = {k: sum(r[k] for r in rows) for k in ('rx', 'tx')}
    projection = {k: round(measured[k] * 30 * 86400 / seconds) for k in ('rx', 'tx')} if seconds else None
    date = interface.get('updated', {}).get('date', {})
    today = next((r for r in traffic.get('day', []) if r['date'] == date), None)
    month = next((r for r in traffic.get('month', [])
                  if r['date'].get('year') == date.get('year') and r['date'].get('month') == date.get('month')), None)
    pair = lambda r: {k: r[k] for k in ('rx', 'tx')} if r else None
    return {'today': pair(today), 'month': pair(month), 'total': pair(traffic.get('total')),
            'recent': measured if seconds else None, 'observed_seconds': seconds,
            'forecast_30d': projection, 'as_of': updated}

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
            'summary': summary, 'latency': latency, 'history': history, 'errors': errors,
            'history_range': [stamp(history[0]['time']), stamp(history[-1]['time'])]}
    data['updated_label'] = stamp(now)
    data['started_label'] = stamp(data['started'])
    temp = ROOT / 'data.json.tmp'
    temp.write_text(json.dumps(data, ensure_ascii=False))
    temp.chmod(0o644)
    temp.replace(path)

if __name__ == '__main__':
    run()
