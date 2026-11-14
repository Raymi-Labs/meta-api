import React from 'react';
import { useTranslation } from 'react-i18next';
import { passwordFeedback } from '../../helpers/passwordFeedback';

export default function PasswordFeedback({
  password = '',
  confirmation,
  maxLength = 20,
  confirmationOnly = false,
}) {
  const { t } = useTranslation();
  const feedback = passwordFeedback(password, confirmation, maxLength);
  if (confirmationOnly ? !confirmation : !password && !confirmation) return null;
  const strengthLabels = [
    t('强度参考：较弱'),
    t('强度参考：一般'),
    t('强度参考：较强'),
  ];
  return (
    <div
      role='status'
      aria-live='polite'
      className='text-xs mt-1 mb-2'
      style={{ color: 'var(--semi-color-text-2)' }}
    >
      {!confirmationOnly && password && (
        <>
          <div
            style={{
              color: feedback.validLength
                ? undefined
                : 'var(--semi-color-danger)',
            }}
          >
            {maxLength == null
              ? t('密码长度至少为8个字符')
              : t('输入密码，最短 8 位，最长 20 位')}
          </div>
          {feedback.validLength && (
            <div>{strengthLabels[feedback.strength]}</div>
          )}
        </>
      )}
      {feedback.matches !== null && (
        <div
          style={{
            color: feedback.matches
              ? 'var(--semi-color-success)'
              : 'var(--semi-color-danger)',
          }}
        >
          {feedback.matches
            ? t('两次输入的密码一致')
            : t('两次输入的密码不一致')}
        </div>
      )}
    </div>
  );
}
