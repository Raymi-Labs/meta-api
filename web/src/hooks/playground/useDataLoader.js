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

import { useEffect, useRef, useState, useCallback } from 'react';
import { useTranslation } from 'react-i18next';
import {
  API,
  processModelsData,
  processGroupsData,
  showError,
} from '../../helpers';
import { API_ENDPOINTS } from '../../constants/playground.constants';

export const useDataLoader = (
  userState,
  inputs,
  handleInputChange,
  setModels,
  setGroups,
) => {
  const { t } = useTranslation();
  const [retry, setRetry] = useState(0);
  const [modelState, setModelState] = useState({
    group: null,
    loading: true,
    error: false,
  });
  const latestInputs = useRef(inputs);
  latestInputs.current = inputs;
  const userId = userState?.user?.id;
  const userGroup = userState?.user?.group;
  useEffect(() => {
    if (!userId) return;
    let active = true;
    const group = inputs.group;
    setModelState({ group, loading: true, error: false });
    API.get(API_ENDPOINTS.USER_MODELS, {
      params: group ? { group } : undefined,
    })
      .then(({ data }) => {
        if (!active) return;
        if (!data.success || !Array.isArray(data.data))
          throw new Error('models unavailable');
        const { modelOptions, selectedModel } = processModelsData(
          data.data,
          latestInputs.current.model,
        );
        setModels(modelOptions);
        if (selectedModel !== latestInputs.current.model)
          handleInputChange('model', selectedModel || '');
        setModelState({ group, loading: false, error: false });
      })
      .catch(() => {
        if (active) {
          setModels([]);
          setModelState({ group, loading: false, error: true });
        }
      });
    return () => {
      active = false;
    };
  }, [userId, inputs.group, retry, setModels, handleInputChange]);
  useEffect(() => {
    if (!userId) return;
    let active = true;
    API.get(API_ENDPOINTS.USER_GROUPS)
      .then(({ data }) => {
        if (!active) return;
        if (!data.success) throw new Error('groups unavailable');
        const options = processGroupsData(data.data, userGroup);
        setGroups(options);
        if (
          !options.some((option) => option.value === latestInputs.current.group)
        )
          handleInputChange('group', options[0]?.value || '');
      })
      .catch(() => {
        if (active) showError(t('加载分组失败'));
      });
    return () => {
      active = false;
    };
  }, [userId, userGroup, retry, setGroups, handleInputChange, t]);
  const reloadModels = useCallback(() => setRetry((value) => value + 1), []);
  return {
    modelsLoading: modelState.loading || modelState.group !== inputs.group,
    modelsError: modelState.error,
    reloadModels,
  };
};
