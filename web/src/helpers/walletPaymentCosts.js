/*
Copyright (C) 2025 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/

export const walletQuoteProvider = (method) =>
  method === 'stripe'
    ? 'stripe'
    : method === 'waffo' || method?.startsWith('waffo:')
      ? 'waffo'
      : 'epay';

export const walletQuoteEndpoint = (method) => {
  const provider = walletQuoteProvider(method);
  return provider === 'epay'
    ? '/api/user/amount'
    : `/api/user/${provider}/amount`;
};

export function getWalletCostReference(
  reference,
  stripeQuote,
  now = Date.now(),
) {
  if (
    !reference ||
    reference.method !== 'stripe' ||
    reference.baseline_method !== 'alipay' ||
    stripeQuote?.currency !== 'USD' ||
    reference.sample_count < 3 ||
    reference.expires_at * 1000 <= now ||
    reference.observed_at * 1000 > now ||
    !Number.isFinite(reference.difference_percent) ||
    ![
      'sample_count',
      'sample_amount',
      'baseline_amount',
      'reference_paid',
      'reference_usd',
      'observed_at',
      'expires_at',
    ].every((key) => Number.isFinite(reference[key]) && reference[key] > 0)
  )
    return null;
  return reference;
}

export const isStripeBelowMinimum = (method, quote) =>
  method === 'stripe' &&
  quote?.currency === 'USD' &&
  Number.isFinite(quote.amount) &&
  quote.amount > 0 &&
  quote.amount < 0.5;
