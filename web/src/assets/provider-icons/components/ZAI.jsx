// Adapted from @lobehub/icons 6.13.0 (MIT). See ../LICENSE.lobehub.
const TITLE = 'Z.ai';
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
        d: 'M12.105 2L9.927 4.953H.653L2.83 2h9.276zM23.254 19.048L21.078 22h-9.242l2.174-2.952h9.244zM24 2L9.264 22H0L14.736 2H24z',
      }),
    ],
  });
});
export { Icon as default };
