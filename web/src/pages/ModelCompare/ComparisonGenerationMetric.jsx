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
import { supportsTextGenerationMetrics } from '../../helpers/modelHealthMetrics';
import { modelPerformance } from '../../helpers/modelPerformance';
import { formatModelThroughput } from '../../helpers/modelPresentation';
import { formatHealthLatency } from '../../components/group-health/healthUtils';
import ModelGroupMetric from '../ModelDetail/ModelGroupMetric';

export default function ComparisonGenerationMetric({
  model,
  health,
  group,
  stats,
  latency,
  t,
}) {
  if (!supportsTextGenerationMetrics(model)) return t('不支持');
  if (!group || !model.enable_groups?.includes(group))
    return t('当前分组不可用');
  const perf = modelPerformance(stats);
  const value = latency ? perf.latency.value : perf.throughput;
  const external = latency ? perf.externalLatency : perf.externalThroughput;
  const historical = latency
    ? perf.latency.referenceTs
    : perf.throughputReferenceTs;
  const formatted = latency
    ? formatHealthLatency(value)
    : formatModelThroughput(value);
  return (
    <ModelGroupMetric
      health={health}
      value={value}
      samples={stats?.request_count}
      formatted={
        external
          ? `${formatted} · ${(latency ? perf.latencyWindow : perf.speedWindow) === 1440 ? t('最近 {{hours}} 小时', { hours: 24 }) : t('近30分钟')} ${t('参考 P50')}`
          : historical
            ? `${formatted} · ${t('最近有效统计')}`
            : formatted
      }
      modelName={model.model_name}
      label={t(latency ? '首字时间' : '输出速度')}
      t={t}
    />
  );
}
