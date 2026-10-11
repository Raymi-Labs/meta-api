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

import React from 'react';
import { SiAlipay, SiWechat, SiStripe } from 'react-icons/si';
import { CreditCard, Coins } from 'lucide-react';
import { getEpayCryptoMethod } from '../../helpers/epayMethods';

const ICONS = {
  alipay: [SiAlipay, '#1677FF'],
  wxpay: [SiWechat, '#07C160'],
  stripe: [SiStripe, '#635BFF'],
  card: [CreditCard, 'var(--semi-color-text-2)'],
};

export default function PaymentMethodIcon({
  method = {},
  size = 18,
  className,
}) {
  const override =
    method.type === 'stripe' &&
    Object.hasOwn(ICONS, method.display_icon) &&
    method.display_icon;
  if (!override && method.icon) {
    return (
      <img
        src={method.icon}
        alt=''
        className={className}
        style={{ width: size, height: size, objectFit: 'contain' }}
      />
    );
  }
  const kind = override || method.type;
  const crypto = getEpayCryptoMethod(method.type);
  const [Icon, color] = (Object.hasOwn(ICONS, kind) ? ICONS[kind] : null) || [
    crypto ? Coins : CreditCard,
    crypto?.color || method.color || 'var(--semi-color-text-2)',
  ];
  return (
    <Icon
      size={size}
      color={color}
      className={className}
      aria-hidden='true'
      data-payment-icon={kind}
    />
  );
}
