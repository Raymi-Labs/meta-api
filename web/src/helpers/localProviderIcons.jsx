import React, { useState } from 'react';
import assets from '../assets/provider-icons/assets.json';

// Vite resolves this fixed set during the build. Database values never become URLs.
const files = import.meta.glob(
  '../assets/provider-icons/images/*.{svg,png,jpeg,jpg,webp}',
  {
    eager: true,
    query: '?url',
    import: 'default',
  },
);

function AssetIcon({
  name,
  asset,
  size = '1em',
  color,
  style,
  className,
  ...props
}) {
  const [failed, setFailed] = useState(false);
  const src = files[`../assets/provider-icons/images/${asset.file}`];
  const common = {
    display: 'inline-block',
    width: size,
    height: size,
    flex: 'none',
    verticalAlign: 'middle',
    ...style,
  };
  const accessibility = {
    role: props.role,
    'aria-label': props['aria-label'],
    'aria-hidden': props['aria-hidden'],
  };
  if (!src || failed)
    return (
      <span className={className} style={common} {...accessibility}>
        {name.charAt(0)}
      </span>
    );
  if (asset.mode === 'mono')
    return (
      <span
        className={className}
        title={name}
        {...accessibility}
        style={{
          ...common,
          backgroundColor: color || 'currentColor',
          mask: `url("${src}") center / contain no-repeat`,
          WebkitMask: `url("${src}") center / contain no-repeat`,
        }}
      />
    );
  return (
    <img
      src={src}
      alt={props['aria-label'] || name}
      className={className}
      {...accessibility}
      draggable={false}
      onError={() => setFailed(true)}
      style={{
        ...common,
        objectFit: 'contain',
        backgroundColor: '#fff',
        borderRadius: 3,
      }}
    />
  );
}

export const localAssetIconRegistry = new Map(
  Object.entries(assets).map(([name, asset]) => [
    `${name}.Mono`,
    async () => ({
      default: (props) => <AssetIcon name={name} asset={asset} {...props} />,
    }),
  ]),
);
