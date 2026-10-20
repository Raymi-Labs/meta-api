import React, { useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';

// Copy only the exact public model ID, never token credentials or restrictions.
export default function TokenModelCopyList({ models }) {
  const { t } = useTranslation();
  const [feedback, setFeedback] = useState('');
  const request = useRef(0);
  const copyModel = async (name) => {
    const id = ++request.current;
    setFeedback('');
    try {
      await navigator.clipboard.writeText(name);
      if (id === request.current)
        setFeedback(`${t('已复制模型名称')}：${name}`);
    } catch {
      if (id === request.current)
        setFeedback(t('复制失败，请手动选择模型名称复制'));
    }
  };
  return (
    <>
      <div className='token-model-copy-list'>
        {models.map(({ value }) => (
          <button
            type='button'
            key={value}
            title={t('点击复制模型名称')}
            aria-label={`${t('复制模型名称')} ${value}`}
            onClick={() => copyModel(value)}
          >
            {value}
          </button>
        ))}
      </div>
      <span
        className='token-model-copy-feedback'
        role='status'
        aria-live='polite'
      >
        {feedback}
      </span>
    </>
  );
}
