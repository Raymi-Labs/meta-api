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

import assert from 'node:assert/strict';
import { mock, test } from 'bun:test';
import React from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { createInstance } from 'i18next';
import { I18nextProvider } from 'react-i18next';
import { StaticRouter } from 'react-router-dom/server';
import SemiSelect from '@douyinfe/semi-ui/lib/es/select/index.js';
import zh from '../src/i18n/locales/zh-CN.json';
import de from '../src/i18n/locales/de.json';

// Isolate network/toast and unrelated layout, while using the real component,
// React translation context, and installed Semi Select update lifecycle.
let selectProps;
const Container = ({ children }) => <>{children}</>;
const CaptureSelect = (props) => {
  selectProps = props;
  return null;
};
CaptureSelect.Option = SemiSelect.Option;
mock.module('@douyinfe/semi-ui', () => ({
  Card: ({ title, headerExtraContent, children }) => (
    <>
      {title}
      {headerExtraContent}
      {children}
    </>
  ),
  Table: () => null,
  Spin: Container,
  Button: Container,
  Select: CaptureSelect,
  Space: Container,
  Tag: Container,
  Typography: { Text: Container },
}));
mock.module('../src/helpers/apiCore', () => ({ API: {} }));
mock.module('../src/helpers/toast', () => ({ showError: () => {} }));
test('monitor range labels update in Semi Select without changing the selected range', async () => {
  const { default: ChannelMonitorPanel } = await import(
    '../src/components/dashboard/ChannelMonitorPanel.jsx'
  );
  const i18n = createInstance();
  await i18n.init({
    lng: 'zh-CN',
    resources: { 'zh-CN': zh, de },
    interpolation: { escapeValue: false },
  });
  const render = () =>
    renderToStaticMarkup(
      <I18nextProvider i18n={i18n}>
        <StaticRouter>
          <ChannelMonitorPanel />
        </StaticRouter>
      </I18nextProvider>,
    );
  render();
  let changed = 0;
  const original = {
    ...SemiSelect.defaultProps,
    ...selectProps,
    onChange: () => changed++,
  };
  const select = new SemiSelect(original);
  // React commits setState for this mounted class; an immediate commit keeps
  // the real Semi adapter/foundation usable without a DOM implementation.
  select.setState = (update, callback) => {
    Object.assign(
      select.state,
      typeof update === 'function'
        ? update(select.state, select.props)
        : update,
    );
    callback?.();
  };
  select.foundation.init();
  assert.equal([...select.state.selections.keys()][0], '最近24小时');
  const values = select.state.options.map((option) => option.value);

  await i18n.changeLanguage('de');
  render();
  select.props = { ...original, ...selectProps, onChange: original.onChange };
  select.componentDidUpdate(original);

  assert.equal(select.props.value, 86400);
  assert.equal([...select.state.selections.keys()][0], 'Letzte 24 Stunden');
  assert.deepEqual(
    select.state.options.map((option) => option.value),
    values,
  );
  assert.deepEqual(
    select.state.options.map((option) => option.label),
    [
      '最近1小时',
      '最近6小时',
      '最近24小时',
      '最近3天',
      '最近7天',
      '最近30天',
    ].map((key) => de.translation[key]),
  );
  assert.equal(
    changed,
    0,
    'translating labels must not trigger a range change',
  );
});
