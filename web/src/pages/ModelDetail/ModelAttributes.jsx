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
import { useTranslation } from 'react-i18next';
import {
  Type,
  Image,
  AudioLines,
  Video,
  FileText,
  Lightbulb,
  Zap,
  Braces,
  Globe,
  Link,
  Code,
  Monitor,
  Search,
  Library,
  CodeXml,
  GitBranch,
  Quote,
  RefreshCw,
  Timer,
  ChevronsRight,
} from 'lucide-react';
import {
  MODEL_MODALITY_LABELS,
  formatModelContextExact,
} from '../../helpers/modelPresentation';
import {
  MODEL_CAPABILITY_LABELS,
  modelCapabilityStatus,
  formatModelTokenLimit,
} from '../../helpers/modelAttributes';

const modalityIcons = {
  text: Type,
  image: Image,
  audio: AudioLines,
  video: Video,
  file: FileText,
};
const capabilityIcons = {
  reasoning: Lightbulb,
  streaming: Zap,
  function_calling: Braces,
  web_search: Globe,
  url_context: Link,
  code_interpreter: Code,
  computer_use: Monitor,
  file_search: Search,
  memory: Library,
  structured_output: CodeXml,
  structured_decision: GitBranch,
  citations: Quote,
  prompt_caching: RefreshCw,
  background: Timer,
  server_side_context: ChevronsRight,
};

export default function ModelAttributes({ model }) {
  const { t } = useTranslation();
  return (
    <section
      id='capabilities'
      className='model-detail-section model-attributes'
      aria-labelledby='model-attributes-title'
    >
      <h2 id='model-attributes-title'>{t('模型属性')}</h2>
      <div className='model-attributes-grid'>
        {[
          ['input_modalities', '输入模态'],
          ['output_modalities', '输出模态'],
        ].map(([field, label]) => {
          const values = (
            Array.isArray(model[field]) ? model[field] : []
          ).filter((value) => MODEL_MODALITY_LABELS[value]);
          return (
            <div className='model-attribute-column' key={field}>
              <h3>{t(label)}</h3>
              {values.length ? (
                <ul>
                  {values.map((value) => {
                    const Icon = modalityIcons[value];
                    return (
                      <li key={value}>
                        <Icon size={19} aria-hidden='true' />
                        {t(MODEL_MODALITY_LABELS[value])}
                      </li>
                    );
                  })}
                </ul>
              ) : (
                <p className='model-attribute-unknown'>—</p>
              )}
            </div>
          );
        })}
        {[
          ['context_length', '上下文'],
          ['max_output_tokens', '最大输出'],
        ].map(([field, label]) => (
          <div className='model-attribute-column' key={field}>
            <h3>{t(label)}</h3>
            <p
              className='model-attribute-limit'
              title={formatModelContextExact(model[field])}
            >
              {formatModelTokenLimit(model[field])}
            </p>
          </div>
        ))}
        <div className='model-attribute-column model-attribute-capabilities'>
          <h3>{t('能力')}</h3>
          <ul className='model-capabilities-list'>
            {Object.entries(MODEL_CAPABILITY_LABELS).map(([key, label]) => {
              const value = model.capabilities?.[key];
              const status = modelCapabilityStatus(value);
              const Icon = capabilityIcons[key];
              return (
                <li
                  key={key}
                  data-capability={key}
                  data-status={status}
                  className={
                    value === true
                      ? 'is-supported'
                      : value === false
                        ? 'is-unsupported'
                        : 'is-unknown'
                  }
                  title={`${t(label)} · ${t(status)}`}
                  aria-label={`${t(label)}：${t(status)}`}
                >
                  <Icon size={19} aria-hidden='true' />
                  <span>{t(label)}</span>
                  {value !== true && <small>{t(status)}</small>}
                </li>
              );
            })}
          </ul>
        </div>
      </div>
    </section>
  );
}
