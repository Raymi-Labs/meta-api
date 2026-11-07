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
import React, { useEffect, useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  prioritizeTokenGroups,
  sortTokenGroupsByRatio,
  tokenGroupPriceLabel,
} from './tokenGroupUtils';
import {
  favoriteStorageKey,
  readGroupFavorites,
  toggleGroupFavorite,
} from './groupFavoriteStorage';
import {
  buildVendorSections,
  nextTokenGroupSelection,
  moveTokenGroup,
  sortSelectedGroupsByRatio,
} from './tokenGroupSelection';
import './tokenGroupPicker.css';

const ALL = '';
const FAVORITES = '__favorites__';

// Vendor-first group selection. The vendor column only filters the group list;
// the ordered selection on the right is what the token stores. In single mode
// (sellable tokens) the selection holds at most one group and cannot be
// reordered.
export default function TokenGroupMultiPicker({
  groups,
  value,
  onChange,
  multiple = true,
  catalog,
  catalogLoading,
  catalogError,
  onCatalogRetry,
  modelVendors,
  userId,
  loading,
  error,
  onRetry,
  footer,
  details,
}) {
  const { t } = useTranslation();
  const selected = Array.isArray(value) ? value : [];
  const [vendorFilter, setVendorFilter] = useState(ALL);
  const [vendorSearch, setVendorSearch] = useState('');
  const [groupSearch, setGroupSearch] = useState('');
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

  const byName = useMemo(
    () => new Map((groups || []).map((group) => [group.value, group])),
    [groups],
  );
  const allSections = useMemo(
    () => buildVendorSections(catalog || {}, modelVendors || {}, '', t('其他')),
    [catalog, modelVendors, t],
  );
  const sections = useMemo(
    () =>
      vendorSearch.trim()
        ? buildVendorSections(
            catalog || {},
            modelVendors || {},
            vendorSearch,
            t('其他'),
          )
        : allSections,
    [allSections, catalog, modelVendors, vendorSearch, t],
  );
  const activeVendor =
    vendorFilter && vendorFilter !== FAVORITES
      ? allSections.find((section) => section.vendor === vendorFilter)
      : null;
  const vendorGroups = activeVendor ? activeVendor.groups : null;
  const sortedGroups = useMemo(
    () => prioritizeTokenGroups(sortTokenGroupsByRatio(groups || [])),
    [groups],
  );
  const query = groupSearch.trim().toLocaleLowerCase();
  const visibleGroups = sortedGroups.filter((group) => {
    if (vendorFilter === FAVORITES && !favorites.includes(group.value))
      return false;
    if (vendorGroups && !vendorGroups.has(group.value)) return false;
    return `${group.value} ${group.label || ''}`
      .toLocaleLowerCase()
      .includes(query);
  });

  const toggle = (name) =>
    onChange(nextTokenGroupSelection(selected, name, multiple));
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
    if (!group || group.value === 'auto') return t('动态');
    return typeof group.ratio === 'number' &&
      Number.isFinite(group.ratio) &&
      group.ratio >= 0
      ? `${group.ratio}×`
      : t('暂不可用');
  };
  const covered =
    vendorGroups && selected.filter((name) => vendorGroups.has(name)).length;

  return (
    <div className='token-group-multi-wrap'>
      <section
        className='token-group-multi'
        aria-label={t('令牌分组')}
        data-testid='token-group-multi-picker'
      >
        <nav className='token-group-multi__models' aria-label={t('按厂商筛选')}>
          <input
            className='token-group-picker__search'
            aria-label={t('搜索厂商或模型')}
            placeholder={t('搜索厂商或模型')}
            value={vendorSearch}
            onChange={(event) => setVendorSearch(event.target.value)}
          />
          <div className='token-group-multi__model-list'>
            <button
              type='button'
              aria-pressed={vendorFilter === ALL}
              onClick={() => setVendorFilter(ALL)}
            >
              {t('全部厂商')}
            </button>
            <button
              type='button'
              aria-pressed={vendorFilter === FAVORITES}
              onClick={() => setVendorFilter(FAVORITES)}
            >
              {`☆ ${t('我的收藏分组')}`}
            </button>
            {catalogLoading ? (
              <p role='status'>{t('加载中...')}</p>
            ) : catalogError ? (
              <p role='alert'>
                {t('模型加载失败')}{' '}
                <button type='button' onClick={onCatalogRetry}>
                  {t('重试')}
                </button>
              </p>
            ) : (
              sections.map((section) => (
                <button
                  type='button'
                  key={section.vendor}
                  aria-pressed={vendorFilter === section.vendor}
                  onClick={() => setVendorFilter(section.vendor)}
                  className='token-group-multi__vendor-button'
                >
                  <span className='token-group-multi__vendor-row'>
                    <span className='token-group-multi__model-name'>
                      {section.vendor}
                    </span>
                    <span
                      className='token-group-multi__count'
                      title={t('支持该厂商模型的分组数')}
                    >
                      {section.groups.size}
                    </span>
                  </span>
                  {section.matchedModels.length > 0 && (
                    <span className='token-group-multi__matched'>
                      {section.matchedModels.slice(0, 3).join('、')}
                      {section.matchedModels.length > 3 ? ' …' : ''}
                    </span>
                  )}
                </button>
              ))
            )}
            {!catalogLoading &&
              !catalogError &&
              !sections.length &&
              vendorSearch.trim() && <p>{t('暂无匹配厂商或模型')}</p>}
          </div>
        </nav>

        <div className='token-group-picker__main token-group-multi__groups'>
          <input
            className='token-group-picker__search'
            aria-label={t('搜索分组')}
            placeholder={t('搜索分组名称或说明')}
            value={groupSearch}
            onChange={(event) => setGroupSearch(event.target.value)}
          />
          {vendorGroups && (
            <div className='token-group-multi__filter-note'>
              {t('支持 {{vendor}} 模型的分组：{{count}} 个', {
                vendor: vendorFilter,
                count: vendorGroups.size,
              })}
            </div>
          )}
          <div className='token-group-picker__head'>
            <span>{multiple ? t('勾选分组（可多选）') : t('令牌分组')}</span>
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
            ) : !visibleGroups.length ? (
              <p>{t('暂无匹配分组')}</p>
            ) : (
              visibleGroups.map((group) => {
                const description =
                  typeof group.label === 'string' ? group.label : '';
                const favorite = favorites.includes(group.value);
                const priceLabel = tokenGroupPriceLabel(group, t);
                const checked = selected.includes(group.value);
                const order = selected.indexOf(group.value);
                return (
                  <div
                    key={group.value}
                    className={`token-group-picker__row${checked ? ' is-selected' : ''}`}
                    onClick={(event) => {
                      if (
                        event.target.closest('button, a, input, label, summary')
                      )
                        return;
                      if (window.getSelection()?.toString()) return;
                      toggle(group.value);
                    }}
                  >
                    <label className='token-group-picker__choice'>
                      <input
                        type={multiple ? 'checkbox' : 'radio'}
                        name='token-group-choice'
                        aria-label={group.value}
                        value={group.value}
                        checked={checked}
                        onChange={() => toggle(group.value)}
                      />
                      <span className='token-group-picker__copy'>
                        <strong>{group.value}</strong>
                        {multiple && checked && (
                          <span className='token-group-multi__order-badge'>
                            {order === 0 ? t('首选') : `#${order + 1}`}
                          </span>
                        )}
                        {priceLabel && (
                          <span className='token-group-picker__price-label'>
                            {priceLabel}
                          </span>
                        )}
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
                    {description && (
                      <span className='token-group-picker__description is-clamped'>
                        {description}
                      </span>
                    )}
                  </div>
                );
              })
            )}
          </div>
          {storageError && (
            <p role='status'>{t('无法保存收藏，关闭页面后可能丢失')}</p>
          )}
        </div>

        <aside className='token-group-multi__order' aria-label={t('使用顺序')}>
          <div className='token-group-multi__order-head'>
            <strong>{multiple ? t('使用顺序') : t('已选分组')}</strong>
            {multiple && selected.length > 1 && (
              <button
                type='button'
                className='token-group-multi__link'
                onClick={() =>
                  onChange(sortSelectedGroupsByRatio(selected, byName))
                }
              >
                {t('便宜的排前面')}
              </button>
            )}
          </div>
          {!selected.length ? (
            <p className='token-group-multi__empty'>
              {multiple ? t('在中间勾选至少一个分组') : t('请选择令牌分组')}
            </p>
          ) : (
            <ol className='token-group-multi__order-list'>
              {selected.map((name, index) => {
                const group = byName.get(name);
                return (
                  <li key={name}>
                    <span className='token-group-multi__index'>
                      {index + 1}
                    </span>
                    <span className='token-group-multi__order-name'>
                      <span>
                        {name}
                        {multiple && index === 0 && (
                          <span className='token-group-multi__order-badge'>
                            {t('首选')}
                          </span>
                        )}
                      </span>
                      <small>
                        {group ? ratioText(group) : t('已保存，当前不可选')}
                        {vendorGroups &&
                          !vendorGroups.has(name) &&
                          ` · ${t('不支持 {{vendor}} 模型', { vendor: vendorFilter })}`}
                      </small>
                    </span>
                    {multiple && (
                      <span className='token-group-multi__order-actions'>
                        <button
                          type='button'
                          aria-label={`${t('上移')} ${name}`}
                          disabled={index === 0}
                          onClick={() =>
                            onChange(moveTokenGroup(selected, index, -1))
                          }
                        >
                          ↑
                        </button>
                        <button
                          type='button'
                          aria-label={`${t('下移')} ${name}`}
                          disabled={index === selected.length - 1}
                          onClick={() =>
                            onChange(moveTokenGroup(selected, index, 1))
                          }
                        >
                          ↓
                        </button>
                        <button
                          type='button'
                          aria-label={`${t('移除')} ${name}`}
                          onClick={() =>
                            onChange(selected.filter((item) => item !== name))
                          }
                        >
                          ×
                        </button>
                      </span>
                    )}
                  </li>
                );
              })}
            </ol>
          )}
          {multiple && selected.length > 1 && (
            <p className='token-group-multi__hint'>
              {t('前一个分组不可用时自动换下一个，按实际使用的分组计费。')}
            </p>
          )}
          {multiple && selected.length === 1 && (
            <p className='token-group-multi__hint'>
              {t('可以再勾选其他分组作为备用。')}
            </p>
          )}
          {vendorGroups && selected.length > 0 && (
            <p
              className={`token-group-multi__hint${covered ? '' : ' is-warning'}`}
            >
              {covered
                ? t('已选分组中有 {{count}} 个支持 {{vendor}} 模型', {
                    count: covered,
                    vendor: vendorFilter,
                  })
                : t('已选分组都不支持 {{vendor}} 模型', {
                    vendor: vendorFilter,
                  })}
            </p>
          )}
          {footer}
          {selected[0] && details && (
            <details className='token-group-picker__details' key={selected[0]}>
              <summary>{t('查看首选分组详情')}</summary>
              {details(selected[0])}
            </details>
          )}
        </aside>
      </section>
    </div>
  );
}
