import React, { useEffect, useMemo, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  buildTokenGroupVendorOptions,
  resolveTokenGroupVendor,
} from './tokenGroupUtils';
import {
  favoriteStorageKey,
  readGroupFavorites,
  toggleGroupFavorite,
} from './groupFavoriteStorage';
import './tokenGroupPicker.css';

function GroupDescription({ text }) {
  const { t } = useTranslation();
  const ref = useRef(null);
  const [expanded, setExpanded] = useState(false);
  const [overflow, setOverflow] = useState(false);
  useEffect(() => {
    const element = ref.current;
    const measure = () => {
      if (!expanded)
        setOverflow(element.scrollHeight > element.clientHeight + 1);
    };
    const observer = new ResizeObserver(measure);
    observer.observe(element);
    measure();
    return () => observer.disconnect();
  }, [text, expanded]);
  return (
    <div className='token-group-picker__description-wrap'>
      <span
        ref={ref}
        className={`token-group-picker__description${expanded ? '' : ' is-clamped'}`}
      >
        {text}
      </span>
      {(overflow || expanded) && (
        <button
          type='button'
          className='token-group-picker__expand'
          aria-expanded={expanded}
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? t('收起说明') : t('展开说明')}
        </button>
      )}
    </div>
  );
}

// Filters are presentation state. Only selecting a radio changes the token group.
export default function TokenGroupPicker({
  groups,
  value,
  userId,
  loading,
  error,
  onRetry,
  onSelect,
  details,
}) {
  const { t } = useTranslation();
  const [vendor, setVendor] = useState(() => resolveTokenGroupVendor(value));
  const [search, setSearch] = useState('');
  const [detailsOpen, setDetailsOpen] = useState(false);
  useEffect(() => setDetailsOpen(false), [value]);
  const [storageError, setStorageError] = useState(false);
  const key = favoriteStorageKey(userId);
  const [saved, setSaved] = useState({ key: null, items: [] });
  const favorites = saved.key === key ? saved.items : [];
  useEffect(() => {
    const read = () =>
      setSaved({ key, items: readGroupFavorites(window.localStorage, key) });
    try {
      read();
    } catch {
      setSaved({ key, items: [] });
    }
    const sync = (event) => {
      if (event.key === key || event.key === null) {
        try {
          read();
        } catch {
          /* Keep this window's preference. */
        }
      }
    };
    window.addEventListener('storage', sync);
    return () => window.removeEventListener('storage', sync);
  }, [key]);
  const vendors = useMemo(
    () => buildTokenGroupVendorOptions(groups, t('多厂商通用')),
    [groups, t],
  );
  const filtered = groups.filter((group) => {
    const inVendor =
      vendor === '__favorites__'
        ? favorites.includes(group.value)
        : !vendor || resolveTokenGroupVendor(group.value) === vendor;
    const query = search.trim().toLocaleLowerCase();
    return (
      inVendor &&
      `${group.value} ${group.label || ''}`.toLocaleLowerCase().includes(query)
    );
  });
  const toggleFavorite = (group) => {
    if (!key) return;
    const items = toggleGroupFavorite(favorites, group);
    setSaved({ key, items });
    try {
      window.localStorage.setItem(key, JSON.stringify(items));
      setStorageError(false);
    } catch {
      setStorageError(true);
    }
  };
  const ratioText = (group) => {
    if (group.value === 'auto') return t('动态');
    return typeof group.ratio === 'number' &&
      Number.isFinite(group.ratio) &&
      group.ratio >= 0
      ? `${group.ratio}×`
      : t('暂不可用');
  };
  return (
    <section className='token-group-picker' aria-label={t('令牌分组')}>
      <nav className='token-group-picker__vendors' aria-label={t('供应商')}>
        {[
          { value: '', label: t('全部') },
          { value: '__favorites__', label: `☆ ${t('我的收藏分组')}` },
          ...vendors,
        ].map((option) => (
          <button
            type='button'
            key={option.value}
            aria-pressed={vendor === option.value}
            onClick={() => setVendor(option.value)}
          >
            {option.label}
          </button>
        ))}
      </nav>
      <div className='token-group-picker__main'>
        <input
          className='token-group-picker__search'
          aria-label={t('搜索分组')}
          placeholder={t('搜索分组名称或说明')}
          value={search}
          onChange={(event) => setSearch(event.target.value)}
        />
        <div className='token-group-picker__head'>
          <span>{t('令牌分组')}</span>
          <span>{t('适用倍率')}</span>
        </div>
        <div
          className='token-group-picker__list'
          role='group'
          aria-label={t('选择分组')}
          aria-busy={loading}
        >
          {loading ? (
            <p role='status'>{t('加载中...')}</p>
          ) : error ? (
            <p role='alert'>
              {t('分组加载失败')}{' '}
              <button type='button' onClick={onRetry}>
                {t('重试')}
              </button>
            </p>
          ) : !filtered.length ? (
            <p>{t('暂无匹配分组')}</p>
          ) : (
            filtered.map((group) => {
              const description =
                typeof group.label === 'string' ? group.label : '';
              const favorite = favorites.includes(group.value);
              return (
                <div
                  key={group.value}
                  className={`token-group-picker__row${group.value === value ? ' is-selected' : ''}`}
                >
                  <label className='token-group-picker__choice'>
                    <input
                      type='radio'
                      name='token-group-choice'
                      value={group.value}
                      checked={group.value === value}
                      onChange={() => onSelect(group.value)}
                    />
                    <span className='token-group-picker__copy'>
                      <strong>{group.value}</strong>
                    </span>
                  </label>
                  <span className='token-group-picker__ratio'>
                    {ratioText(group)}
                  </span>
                  <button
                    type='button'
                    className='token-group-picker__star'
                    aria-pressed={favorite}
                    disabled={!key}
                    aria-label={`${favorite ? t('取消收藏') : t('收藏当前分组')} ${group.value}`}
                    onClick={() => toggleFavorite(group.value)}
                  >
                    {favorite ? '★' : '☆'}
                  </button>
                  {description && <GroupDescription text={description} />}
                </div>
              );
            })
          )}
        </div>
        <div className='token-group-picker__selection'>
          <span>
            {value ? `${t('已选分组')}：${value}` : t('请选择令牌分组')}
          </span>
          {value &&
            !loading &&
            !error &&
            !groups.some((group) => group.value === value) && (
              <span>{t('已保存，当前不可选')}</span>
            )}
        </div>
        {value && details && (
          <details
            className='token-group-picker__details'
            key={value}
            onToggle={(event) => setDetailsOpen(event.currentTarget.open)}
          >
            <summary>{t('查看详情')}</summary>
            {detailsOpen && details}
          </details>
        )}
        {storageError && (
          <p role='status'>{t('无法保存收藏，关闭页面后可能丢失')}</p>
        )}
      </div>
    </section>
  );
}
