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
const componentPath =
  /(?:^|\/)@lobehub\/icons\/es\/([^/]+)\/components\/(Mono|Avatar|Brand|BrandColor|Color|Combine|Text|TextCn|TextColor|Simple|Morden)\.js$/;

export function createProviderIconRegistry(loaders) {
  const registry = new Map();
  for (const [path, loader] of Object.entries(loaders)) {
    const match = path.replaceAll('\\', '/').match(componentPath);
    if (match) registry.set(`${match[1]}.${match[2]}`, loader);
  }
  return registry;
}

// Only build-time enumerated, checked-in components may extend the package.
export function createLocalProviderIconRegistry(loaders) {
  const registry = new Map();
  for (const [path, loader] of Object.entries(loaders)) {
    const match = path
      .replaceAll('\\', '/')
      .match(
        /(?:^|\/)assets\/provider-icons\/components\/([A-Za-z][A-Za-z0-9]*)\.jsx$/,
      );
    if (match) registry.set(`${match[1]}.Mono`, loader);
  }
  return registry;
}

export function resolveProviderIcon(registry, name, variant = 'Mono') {
  const preferred = `${name}.${variant}`;
  if (registry.has(preferred)) return preferred;
  const fallback = `${name}.Mono`;
  return registry.has(fallback) ? fallback : null;
}
