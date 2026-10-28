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
import { Avatar, Typography } from '@douyinfe/semi-ui';
import { IconLink } from '@douyinfe/semi-icons';
import { getModelEndpointRows } from '../../../../../helpers/modelProtocols';
import '../../modelProtocols.css';

const { Text } = Typography;

const ModelEndpoints = ({ modelData, endpointMap = {}, t }) => {
  if (!modelData) return null;
  const rows = getModelEndpointRows(modelData, endpointMap);
  const declared = rows.filter((row) => row.declared);
  const templates = rows.filter((row) => !row.declared);
  const renderRow = (row) => (
    <div
      key={row.endpoint}
      className='model-endpoint-row'
      data-protocol={row.endpoint}
      data-declared={row.declared}
    >
      <div className='model-endpoint-label'>
        <span>{row.label}</span>
        <span className='model-endpoint-state'>
          {row.declared ? t('已声明协议') : `${t('示例')} · ${t('未声明')}`}
        </span>
      </div>
      {row.path && (
        <div className='model-endpoint-path'>
          <span className='model-endpoint-method'>{row.method}</span>
          <code>{row.path}</code>
        </div>
      )}
    </div>
  );

  return (
    <div>
      <div className='flex items-center mb-4'>
        <Avatar size='small' color='purple' className='mr-2 shadow-md'>
          <IconLink size={16} />
        </Avatar>
        <div>
          <Text className='text-lg font-medium'>{t('API端点')}</Text>
          <p className='model-endpoint-note'>
            {t(
              '协议声明来自模型目录，不代表所有分组均已实测。未声明不等于不支持，调用前请确认所用分组。',
            )}
          </p>
        </div>
      </div>
      {declared.length ? (
        declared.map(renderRow)
      ) : (
        <p className='model-endpoint-note'>{t('未声明')}</p>
      )}
      {templates.length > 0 && (
        <>
          <h3 className='model-endpoint-heading'>{t('接入示例（未声明）')}</h3>
          <p className='model-endpoint-note'>
            {t('该模型尚未声明支持此协议，请先确认所用分组的接口配置。')}
          </p>
          {templates.map(renderRow)}
        </>
      )}
    </div>
  );
};

export default ModelEndpoints;
