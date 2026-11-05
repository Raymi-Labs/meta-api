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

import { useEffect, useState } from 'react';
import { API } from '../../helpers/apiCore';
import { createPaymentQuote } from '../../helpers/paymentCurrency';
import {
  walletQuoteEndpoint,
  walletQuoteProvider,
} from '../../helpers/walletPaymentCosts';

// Comparison reads never authorize a payment; confirmation keeps its own quote.
export function useWalletComparisonQuotes(amount, methods, enabled) {
  const [state, setState] = useState({ amount: null, quotes: {} });
  useEffect(() => {
    let active = true;
    const abort = new AbortController();
    setState({ amount, quotes: {} });
    if (!enabled || !Number.isInteger(Number(amount)) || Number(amount) <= 0)
      return;
    const providers = new Map();
    for (const method of methods) {
      if (Number(method.min_topup || 0) <= amount) {
        providers.set(walletQuoteProvider(method.type), method.type);
      }
    }
    const timer = setTimeout(() => {
      for (const [provider, method] of providers) {
        API.post(
          walletQuoteEndpoint(method),
          { amount: Number(amount) },
          {
            signal: abort.signal,
            timeout: 10000,
            skipErrorHandler: true,
            skipGlobalLoading: true,
          },
        )
          .then(({ data }) => {
            const quote =
              data?.message === 'success'
                ? createPaymentQuote(
                    data.data,
                    data.currency,
                    method,
                    data.estimated,
                  )
                : null;
            if (active)
              setState((previous) => ({
                amount,
                quotes: { ...previous.quotes, [provider]: quote },
              }));
          })
          .catch(() => {
            if (active)
              setState((previous) => ({
                amount,
                quotes: { ...previous.quotes, [provider]: null },
              }));
          });
      }
    }, 400);
    return () => {
      active = false;
      clearTimeout(timer);
      abort.abort();
    };
  }, [amount, methods, enabled]);
  return state.amount === amount ? state.quotes : {};
}
