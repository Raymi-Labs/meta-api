import assert from 'node:assert/strict';
import { test } from 'bun:test';
import fs from 'node:fs';
import { officialPricingUnavailable } from '../src/helpers/modelOfficialPricing';
import { groupComparisonUnavailable } from '../src/helpers/modelGroupTiers';
test('all audited empty states agree across consumers and every locale', () => {
  const reviews = JSON.parse(
    fs.readFileSync(
      new URL(
        '../../model/data/official_pricing_reviews.json',
        import.meta.url,
      ),
    ),
  );
  assert.equal(Object.keys(reviews).length, 40);
  for (const review of Object.values(reviews)) {
    const message = officialPricingUnavailable(review);
    assert.equal(
      groupComparisonUnavailable({ official_pricing_review: review }),
      message,
    );
    for (const file of fs.readdirSync(
      new URL('../src/i18n/locales', import.meta.url),
    )) {
      const locale = JSON.parse(
        fs.readFileSync(
          new URL('../src/i18n/locales/' + file, import.meta.url),
        ),
      );
      assert.ok(locale.translation[message], file + ': ' + message);
      assert.ok(locale.translation['最近核实：{{date}}'].includes('{{date}}'));
    }
  }
});
test('unknown review codes have a conservative fallback', () => {
  assert.equal(
    officialPricingUnavailable({ code: 'unknown-new-code' }),
    '官方参考价待核实',
  );
  assert.equal(officialPricingUnavailable(undefined), '官方参考价待核实');
});
