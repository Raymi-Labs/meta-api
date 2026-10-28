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
import { Link } from 'react-router-dom';
import {
  getDeclaredModelProtocols,
  getModelProtocolLabel,
  canOfferTextProtocolExamples,
} from '../../../helpers/modelProtocols';
import { buildModelDetailPath } from '../../../helpers/modelDetailNavigation';
import './modelProtocols.css';

const ModelProtocolSummary = ({ model, catalogState, t }) => {
  const declared = getDeclaredModelProtocols(model);
  const showResponsesExample =
    canOfferTextProtocolExamples(model) &&
    !declared.includes('openai-response');
  return (
    <div className='model-protocol-summary' aria-label={t('接口协议')}>
      <span className='model-protocol-caption'>
        {t(declared.length ? '已声明协议' : '接口协议')}:
      </span>
      {declared.length ? (
        declared.map((endpoint) => (
          <span key={endpoint} className='model-protocol-tag' title={endpoint}>
            {getModelProtocolLabel(endpoint)}
          </span>
        ))
      ) : (
        <span>{t('未声明')}</span>
      )}
      {showResponsesExample && (
        <Link
          to={`${buildModelDetailPath(model.model_name)}#endpoints`}
          state={{ catalogState }}
          className='model-protocol-template'
          title={t('该模型尚未声明支持此协议，请先确认所用分组的接口配置。')}
          onClick={(event) => event.stopPropagation()}
        >
          Responses · {t('示例')} · {t('未声明')}
        </Link>
      )}
    </div>
  );
};

export default ModelProtocolSummary;
