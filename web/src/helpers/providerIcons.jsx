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
import React, { lazy, Suspense } from 'react';
import {
  createProviderIconRegistry,
  createLocalProviderIconRegistry,
  resolveProviderIcon,
} from './providerIconRegistry';
import { localAssetIconRegistry } from './localProviderIcons';

// Vite enumerates these at build time. There is no arbitrary runtime import path,
// and a provider's Avatar/UI dependencies are not loaded with its Mono/Color SVG.
const registry = createProviderIconRegistry(
  import.meta.glob(
    '../../node_modules/@lobehub/icons/es/*/components/{Mono,Avatar,Brand,BrandColor,Color,Combine,Text,TextCn,TextColor,Simple,Morden}.js',
  ),
);
for (const [key, loader] of [
  ...createLocalProviderIconRegistry(
    import.meta.glob('../assets/provider-icons/components/*.jsx'),
  ),
  ...localAssetIconRegistry,
]) {
  if (!registry.has(key)) registry.set(key, loader);
}
const components = new Map();

function IconFallback({ name, size = 16, className, style, ...props }) {
  return (
    <span
      className={className}
      role={props.role}
      aria-label={props['aria-label']}
      aria-hidden={props['aria-hidden']}
      style={{
        display: 'inline-flex',
        alignItems: 'center',
        justifyContent: 'center',
        width: size,
        height: size,
        flexShrink: 0,
        ...style,
      }}
    >
      {name?.charAt(0).toUpperCase() || '?'}
    </span>
  );
}

export function getProviderIconComponent(name, variant = 'Mono') {
  const key = resolveProviderIcon(registry, name, variant);
  if (!key) return null;
  if (!components.has(key)) {
    const LazyIcon = lazy(() =>
      registry
        .get(key)()
        .catch(() => ({
          default: (props) => <IconFallback name={name} {...props} />,
        })),
    );
    const Icon = (props) => (
      <Suspense fallback={<IconFallback name={name} {...props} />}>
        <LazyIcon {...props} />
      </Suspense>
    );
    components.set(key, Icon);
  }
  return components.get(key);
}

function provider(name) {
  const Icon = (props) => {
    const Component = getProviderIconComponent(name);
    return Component ? (
      <Component {...props} />
    ) : (
      <IconFallback name={name} {...props} />
    );
  };
  Icon.Color = (props) => {
    const Component = getProviderIconComponent(name, 'Color');
    return Component ? (
      <Component {...props} />
    ) : (
      <IconFallback name={name} {...props} />
    );
  };
  return Icon;
}

export const OpenAI = provider('OpenAI');
export const Claude = provider('Claude');
export const Gemini = provider('Gemini');
export const Moonshot = provider('Moonshot');
export const Zhipu = provider('Zhipu');
export const Qwen = provider('Qwen');
export const DeepSeek = provider('DeepSeek');
export const Minimax = provider('Minimax');
export const Wenxin = provider('Wenxin');
export const Spark = provider('Spark');
export const Midjourney = provider('Midjourney');
export const Hunyuan = provider('Hunyuan');
export const Cohere = provider('Cohere');
export const Cloudflare = provider('Cloudflare');
export const Ai360 = provider('Ai360');
export const Yi = provider('Yi');
export const Jina = provider('Jina');
export const Mistral = provider('Mistral');
export const XAI = provider('XAI');
export const Ollama = provider('Ollama');
export const Doubao = provider('Doubao');
export const Suno = provider('Suno');
export const Xinference = provider('Xinference');
export const OpenRouter = provider('OpenRouter');
export const Dify = provider('Dify');
export const Coze = provider('Coze');
export const SiliconCloud = provider('SiliconCloud');
export const FastGPT = provider('FastGPT');
export const Kling = provider('Kling');
export const Jimeng = provider('Jimeng');
export const Perplexity = provider('Perplexity');
export const Replicate = provider('Replicate');
export const Grok = provider('Grok');
