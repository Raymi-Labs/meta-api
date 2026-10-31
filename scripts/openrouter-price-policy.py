#!/usr/bin/env python3
"""Compile reviewed OpenRouter quotes into existing channel overrides; no network or credentials.

Usage: python3 scripts/openrouter-price-policy.py AUDIT.json CHANNELS.json OUTPUT.json
The output is a plan, not an API writer. Production writes must re-read current channels.
"""
import json
import sys
from collections import defaultdict
from pathlib import Path


def condition(path, value, mode='full', **kwargs):
    return dict(path=path, mode=mode, value=value, **kwargs)


def op(mode, path='', value=None, conditions=None, **kwargs):
    result = dict(mode=mode, path=path, **kwargs)
    if mode != 'delete':
        result['value'] = value
    if conditions:
        result.update(conditions=conditions, logic='AND')
    return result


def deny(message, conditions=None, code='openrouter_price_policy', status=400):
    return op('return_error', value=dict(message=message, code=code,
              status_code=status, skip_retry=True), conditions=conditions)


def compile_override(rows):
    enabled = [r for r in rows if r['status'] == 'enabled']
    operations = [op('set', 'provider.sort', 'price'),
                  op('set', 'provider.allow_fallbacks', True)]
    operations.append(deny(
        '该模型尚未通过原厂价格审核，OpenRouter 调用已暂停。',
        [condition('model', r['upstream_model'], invert=True, pass_missing_key=True)
         for r in enabled], status=503, code='openrouter_price_unverified'))
    if not enabled:
        return {'operations': operations}

    # Reject optional charge paths, rather than silently run a paid hosted tool.
    # Ordinary client-executed function tools remain usable.
    for path in ['provider.only', 'provider.options', 'web_search_options', 'preset']:
        operations.append(deny('该限价渠道暂不支持自定义 ' + path + '。',
                               [condition(path, None, invert=True)]))
    operations.append(deny('该限价渠道暂不支持额外收费插件或原生托管工具。',
        [condition('plugins.#', 0, mode='gt')]))
    operations.append(deny('该限价渠道仅支持客户端执行的 function tools。',
        [condition('tools.#(type!="function")#.type', [], invert=True)]))
    operations.append(deny('该限价渠道暂不支持跨模型 fallback。',
        [condition('models.#', 0, mode='gt')]))
    operations.extend([op('delete', 'provider.order'), op('delete', 'models'),
                       op('delete', 'speed'), op('set', 'service_tier', 'default'),
                       op('set', 'plugins', [])])
    for row in enabled:
        price = row['official_price']
        for k in ['prompt', 'completion']:
            assert isinstance(price[k], (int, float)) and price[k] >= 0
        assert row['providers'] and row['sources']
        match = condition('model', row['upstream_model'])
        operations.append(op('set', 'provider.only', row['providers'], [match]))
        # Preserve a caller's stricter limit, including zero. Invalid types fail
        # in the existing numeric comparator before any upstream request.
        for key, limit in dict(prompt=price['prompt'], completion=price['completion'],
                               request=0, image=0).items():
            path = 'provider.max_price.' + key
            operations.append(op('set', path, limit, [match,
                condition(path, limit, mode='gt', pass_missing_key=True)]))
    return {'operations': operations}


def build_plan(audit, channels):
    if isinstance(channels, dict):
        channels = channels.get('channels', channels.get('data'))
    current = {c['id']: c for c in channels}
    grouped = defaultdict(list)
    for row in audit['models']:
        assert row['status'] in ['enabled', 'paused']
        grouped[row['channel_id']].append(row)
    plan = []
    for cid, rows in sorted(grouped.items()):
        old = current[cid]
        assert old['status'] == 1 and old['type'] == 20, cid
        assert not old.get('param_override'), 'Merge existing overrides explicitly'
        assert set(old['models'].split(',')) == {r['model'] for r in rows}
        mapping = json.loads(old.get('model_mapping') or '{}') or {}
        assert all(mapping.get(r['model'], r['model']) == r['upstream_model'] for r in rows)
        models = [r['model'] for r in rows if r['status'] == 'enabled']
        patch = dict(id=cid, type=20, status=1 if models else 2,
                     models=','.join(models) if models else old['models'],
                     param_override=json.dumps(compile_override(rows), ensure_ascii=False, separators=(',', ':')))
        expected = {k: old.get(k) for k in ['id', 'type', 'name', 'status', 'models',
                    'model_mapping', 'param_override', 'base_url', 'group']}
        plan.append(dict(expected=expected, patch=patch,
                         paused_models=[r['model'] for r in rows if r['status']=='paused']))
    return dict(reviewed_at=audit['reviewed_at'], channels=plan,
                enabled_models=sum(r['status']=='enabled' for r in audit['models']),
                paused_models=sum(r['status']=='paused' for r in audit['models']))


if __name__ == '__main__':
    if len(sys.argv) != 4:
        raise SystemExit(__doc__)
    audit, channels = [json.loads(Path(p).read_text()) for p in sys.argv[1:3]]
    plan = build_plan(audit, channels)
    Path(sys.argv[3]).write_text(json.dumps(plan, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({k: v for k, v in plan.items() if k != 'channels'}, ensure_ascii=False))
