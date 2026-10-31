#!/usr/bin/env python3
"""Offline retail correction plan; no credentials, network or database writes.

Usage: openrouter-retail-plan.py OLD_AUDIT GEMINI_AUDIT SNAPSHOT OUTPUT
Rates are energy/MTok = manufacturer USD/MTok * public USD/CNY / CNY per energy.
Existing lower component prices and per-request prices are preserved.
"""
import json
import sys
from decimal import Decimal, ROUND_DOWN
from pathlib import Path


def number(value):
    return Decimal(str(value))


def rounded(value):
    return float(value.quantize(Decimal('0.000000000001'), rounding=ROUND_DOWN))


def build_plan(old_audit, gemini_audit, snapshot):
    fx = number(gemini_audit['exchange_rate']['usd_cny'])
    options = {k: json.loads(v) for k, v in snapshot['options'].items() if v}
    recharge = number(options['Price'])
    assert recharge > 0 and fx > 0
    factor = fx / recharge
    changes = {}
    rows = []
    expressions = {}

    def update(key, name, value):
        old = options.get(key, {}).get(name)
        if old != value:
            changes.setdefault(key, {})[name] = {'before': old, 'after': value}

    targets = [r for r in old_audit['models'] if r['status'] == 'enabled']
    targets += [r for r in gemini_audit['models'] if r['price_verified']]
    for row in targets:
        name, price = row['model'], row['official_price']
        if name in options.get('ModelPrice', {}):
            # Do not silently replace a pre-existing flat-price contract shared
            # by other channels. It remains pending for explicit later review.
            rows.append({'model': name, 'action': 'preserve_existing_fixed_price', 'price': options['ModelPrice'][name]})
            continue
        current = options.get('ModelRatio', {}).get(name)
        old_in = number(current) * 2 if current is not None else None
        cap_in = number(price['prompt']) * factor
        new_in = min(old_in, cap_in) if old_in is not None else cap_in
        assert new_in > 0
        old_out = old_in * number(options.get('CompletionRatio', {}).get(name, 1)) if old_in is not None else None
        cap_out = number(price['completion']) * factor
        new_out = min(old_out, cap_out) if old_out is not None else cap_out
        update('ModelRatio', name, rounded(new_in / 2))
        # Divide by the stored input price, not an unrounded intermediate.
        stored_in = number(changes.get('ModelRatio', {}).get(name, {}).get('after', current)) * 2
        update('CompletionRatio', name, rounded(new_out / stored_in))
        effective = {'prompt': stored_in, 'completion': stored_in * number(changes.get('CompletionRatio', {}).get(name, {}).get('after', options.get('CompletionRatio', {}).get(name, 1)))}
        if price.get('input_cache_read') is not None:
            old_cache = old_in * number(options.get('CacheRatio', {}).get(name, 1)) if old_in is not None else None
            cap_cache = number(price['input_cache_read']) * factor
            cache = min(old_cache, cap_cache) if old_cache is not None else cap_cache
            update('CacheRatio', name, rounded(cache / stored_in))
            effective['input_cache_read'] = stored_in * number(changes.get('CacheRatio', {}).get(name, {}).get('after', options.get('CacheRatio', {}).get(name, 1)))
        if row.get('channel_id') == 1638:
            modalities = row['output_modalities']
            if 'speech' in modalities:
                update('AudioRatio', name, 1)
                update('AudioCompletionRatio', name, rounded(new_out / stored_in))
            elif 'transcription' in modalities:
                update('AudioRatio', name, 1)
            elif 'image_output' in price:
                image = number(price['image_output']) * factor
                if old_out is not None:
                    image = min(old_out, image)
                # Actual image output tokens are separately normalized by the
                # existing billing expression engine; no double text charge.
                expr = f'tier("standard", p * {rounded(stored_in)} + c * {rounded(effective["completion"])} + img_o * {rounded(image)})'
                cache = effective.get('input_cache_read')
                if cache is None and old_in is not None and name in options.get('CacheRatio', {}):
                    cache = min(old_in, stored_in) * number(options['CacheRatio'][name])
                if cache is not None:
                    expr = expr[:-1] + f' + cr * {rounded(cache)})'
                expressions[name] = expr
                effective['image_output'] = number(rounded(image))
            elif 'audio' in price:
                old_audio = old_in * number(options.get('AudioRatio', {}).get(name, 1)) if old_in is not None else None
                cap_audio = number(price['audio']) * factor
                ai = min(old_audio, cap_audio) if old_audio is not None else cap_audio
                update('AudioRatio', name, rounded(ai / stored_in))
        for field, value in effective.items():
            assert value <= number(price[field]) * factor + Decimal('0.00000000001'), (name, field)
        rows.append({'model': name, 'action': 'lower_or_preserve_components', 'manufacturer_usd_per_million': price, 'energy_per_million': {k:float(v) for k,v in effective.items()}, 'previous_input_energy_per_million':float(old_in) if old_in is not None else None})
    return {'exchange_rate': gemini_audit['exchange_rate'], 'cny_per_energy': float(recharge), 'energy_per_usd': float(factor), 'option_changes': changes, 'billing_expressions': expressions, 'models': rows}


if __name__ == '__main__':
    if len(sys.argv) != 5:
        raise SystemExit(__doc__)
    plan = build_plan(*[json.loads(Path(p).read_text()) for p in sys.argv[1:4]])
    Path(sys.argv[4]).write_text(json.dumps(plan, ensure_ascii=False, indent=2)+'\n')
    print(json.dumps({'models':len(plan['models']), 'changed_option_entries':sum(len(v) for v in plan['option_changes'].values()), 'expressions':list(plan['billing_expressions']), 'energy_per_usd':plan['energy_per_usd']}))
