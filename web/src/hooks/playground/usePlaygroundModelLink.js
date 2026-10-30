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

import { useEffect, useRef, useState } from 'react';

// A deep link requests a selection; the current account's API lists authorize it.
export function usePlaygroundModelLink({
  searchParams,
  groups,
  models,
  modelsLoading,
  inputs,
  onChange,
  blocked,
  setCustomRequestMode,
}) {
  const model = searchParams.get('model') || '';
  const group = searchParams.get('group') || '';
  const key = JSON.stringify([model, group]);
  const [handled, setHandled] = useState('');
  const [notice, setNotice] = useState(null);
  const groupApplied = useRef('');
  const invalidGroup = useRef(null);
  const pending = Boolean(model) && handled !== key;
  useEffect(() => {
    if (!pending) return;
    if (model.length > 255 || group.length > 255) {
      setNotice('模型链接无效，请重新选择模型');
      return;
    }
    if (blocked) {
      setNotice('完成当前请求或工具回合后，将切换到目标模型');
      return;
    }
    if (!groups.length) return;
    if (
      invalidGroup.current?.key === key &&
      invalidGroup.current.group === inputs.group
    )
      return;
    if (group && groupApplied.current !== key) {
      groupApplied.current = key;
      if (!groups.some((option) => option.value === group)) {
        invalidGroup.current = { key, group: inputs.group };
        setNotice('目标分组不可用，请选择可用分组');
        return;
      }
      if (inputs.group !== group) {
        onChange('group', group);
        return;
      }
    }
    if (modelsLoading) return;
    if (!models.some((option) => option.value === model)) {
      setNotice('目标模型在当前分组不可用，请切换分组或选择其他模型');
      return;
    }
    onChange('model', model);
    onChange('modelTabs', [...new Set([...(inputs.modelTabs || []), model])]);
    setCustomRequestMode(false);
    setHandled(key);
    setNotice(null);
  }, [
    pending,
    key,
    model,
    group,
    blocked,
    groups,
    models,
    modelsLoading,
    inputs.group,
    inputs.modelTabs,
    onChange,
    setCustomRequestMode,
  ]);
  return {
    pending,
    notice: pending ? notice : null,
    dismiss: () => {
      setHandled(key);
      setNotice(null);
    },
  };
}
