// Adapted from @lobehub/icons 6.13.0 (MIT). See ../LICENSE.lobehub.
const TITLE = 'Inception';
import { memo } from 'react';
import { jsx, jsxs } from 'react/jsx-runtime';
const Icon = memo(({ size = '1em', style, ...rest }) => {
  return /* @__PURE__ */ jsxs('svg', {
    fill: 'currentColor',
    fillRule: 'evenodd',
    height: size,
    style: {
      flex: 'none',
      lineHeight: 1,
      ...style,
    },
    viewBox: '0 0 24 24',
    width: size,
    xmlns: 'http://www.w3.org/2000/svg',
    ...rest,
    children: [
      /* @__PURE__ */ jsx('title', { children: TITLE }),
      /* @__PURE__ */ jsx('path', {
        d: 'M14.767 1H7.884L1 7.883v6.884h6.884V7.883h6.883V1zM9.234 23h6.882L23 16.116V9.233h-6.884v6.883H9.234V23z',
      }),
    ],
  });
});
export { Icon as default };
