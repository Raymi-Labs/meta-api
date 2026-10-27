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
export const readTokenRouting = (raw) => {
  try {
    const config = JSON.parse(raw || '{}');
    return {
      groups: Array.isArray(config.groups) ? config.groups : [],
      model_groups: config.model_groups || {},
    };
  } catch {
    return { groups: [], model_groups: {} };
  }
};

export const tokenRoutingPayload = (values) => {
  if (!values.routing_enabled) return '';
  const config = readTokenRouting(values.routing_config);
  const selected = new Set(values.model_limits || []);
  return JSON.stringify({
    groups: config.groups,
    model_groups: Object.fromEntries(
      Object.entries(config.model_groups).filter(([name]) =>
        selected.has(name),
      ),
    ),
  });
};

export const tokenRoutingModelPreview = (values) => {
  if (!values?.routing_enabled) return '';
  return JSON.stringify({
    groups: readTokenRouting(values.routing_config).groups,
  });
};
