import React, { useCallback, useEffect, useRef, useState } from 'react';
import { CalendarDays, ChevronDown, Info, RotateCw } from 'lucide-react';
import { API } from '../../helpers/apiCore';
import './usageCalendar.css';
import { activityWeeks, validActivity } from '../../helpers/usageCalendar';

export default function UsageCalendar({ t, userId, defaultOpen = false }) {
  const [open, setOpen] = useState(defaultOpen);
  const [state, setState] = useState({
    loading: false,
    data: null,
    error: false,
  });
  const [selected, setSelected] = useState(null);
  const pending = useRef(null);
  const generation = useRef(0);
  const scroll = useRef(null);
  const zone = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC';
  useEffect(
    () => () => {
      generation.current++;
      pending.current?.abort();
    },
    [userId],
  );
  useEffect(() => {
    if (state.data && scroll.current)
      scroll.current.scrollLeft = scroll.current.scrollWidth;
  }, [state.data]);
  const load = useCallback(async () => {
    pending.current?.abort();
    const controller = new AbortController();
    pending.current = controller;
    const request = ++generation.current;
    setState({ loading: true, data: null, error: false });
    setSelected(null);
    try {
      const response = await API.get('/api/data/self/activity', {
        params: { timezone: zone },
        headers: { 'New-API-User': userId },
        signal: controller.signal,
        skipGlobalLoading: true,
        disableDuplicate: true,
      });
      if (request !== generation.current || controller.signal.aborted) return;
      if (
        !response.data?.success ||
        !validActivity(response.data.data) ||
        response.data.data.timezone !== zone
      )
        throw new Error('Invalid activity');
      setState({ loading: false, error: false, data: response.data.data });
    } catch {
      if (request === generation.current && !controller.signal.aborted)
        setState({ loading: false, error: true, data: null });
    }
  }, [zone, userId]);
  useEffect(() => {
    if (defaultOpen) void load();
  }, [defaultOpen, load]);
  const data = state.data;
  const weeks = data ? activityWeeks(data.days) : [];
  const peak = data ? Math.max(1, ...data.days.map((d) => d.requests)) : 1;
  const level = (count) =>
    count === 0
      ? 0
      : Math.min(4, 1 + Math.floor((Math.log1p(count) / Math.log1p(peak)) * 3));
  const number = (value) =>
    new Intl.NumberFormat(undefined, { maximumFractionDigits: 1 }).format(
      value,
    );
  return (
    <section className='usage-calendar'>
      <button
        className='usage-calendar-toggle'
        type='button'
        aria-expanded={open}
        aria-controls='personal-activity-body'
        onClick={() => {
          setOpen(!open);
          if (!open && !data && !state.loading) void load();
        }}
      >
        <span>
          <CalendarDays size={18} aria-hidden='true' />
          {t('我的调用日历')}
        </span>
        <span className='usage-calendar-toggle-hint'>
          {t('近 365 天')}
          <ChevronDown
            size={16}
            style={{ transform: open ? 'rotate(180deg)' : undefined }}
          />
        </span>
      </button>
      {open && (
        <div id='personal-activity-body' className='usage-calendar-body'>
          <p className='usage-calendar-note'>
            <Info size={14} aria-hidden='true' />
            {t(
              '基于留存的消费记录，不含退款和补差；历史清理或停记可能导致数据不完整。',
            )}
          </p>
          {state.loading && <p role='status'>{t('正在加载调用日历…')}</p>}
          {state.error && (
            <div role='alert' className='usage-calendar-error'>
              <span>{t('暂时无法加载完整日历，请稍后重试。')}</span>
              <button type='button' onClick={load}>
                {t('重试')}
              </button>
            </div>
          )}
          {data && (
            <>
              <div className='usage-calendar-stats'>
                {[
                  ['最长连续调用', data.longest_streak, '天'],
                  ['日均调用', data.daily_average, '次'],
                  ['周均调用', data.weekly_average, '次'],
                  ['区间调用总数', data.total, '次'],
                ].map(([label, value, unit]) => (
                  <div key={label}>
                    <span>{t(label)}</span>
                    <strong>
                      {number(value)} <small>{t(unit)}</small>
                    </strong>
                  </div>
                ))}
              </div>
              {!data.total && (
                <p className='usage-calendar-empty'>
                  {t('近 365 天没有留存的调用记录')}
                </p>
              )}
              <div
                className='usage-calendar-scroll'
                ref={scroll}
                tabIndex={0}
                aria-label={t('调用日期与次数')}
              >
                <div className='usage-calendar-grid'>
                  {weeks.map((week, i) => (
                    <div className='usage-calendar-week' key={i}>
                      <span className='usage-calendar-month'>
                        {week
                          .find(
                            (d) =>
                              d &&
                              (d.date.endsWith('-01') || d === data.days[0]),
                          )
                          ?.date.slice(0, 7) || ''}
                      </span>
                      {week.map((day, j) =>
                        day ? (
                          <button
                            type='button'
                            className={`usage-calendar-day activity-${level(day.requests)}`}
                            key={day.date}
                            title={`${day.date} · ${number(day.requests)} ${t('次')}`}
                            aria-label={`${day.date} · ${number(day.requests)} ${t('次')}`}
                            aria-pressed={selected?.date === day.date}
                            tabIndex={
                              (selected?.date || data.days[364].date) ===
                              day.date
                                ? 0
                                : -1
                            }
                            onKeyDown={(event) => {
                              const delta = {
                                ArrowUp: -1,
                                ArrowDown: 1,
                                ArrowLeft: -7,
                                ArrowRight: 7,
                              }[event.key];
                              if (!delta) return;
                              event.preventDefault();
                              const index = data.days.findIndex(
                                (item) => item.date === day.date,
                              );
                              const cells = scroll.current?.querySelectorAll(
                                '.usage-calendar-day',
                              );
                              cells?.[
                                Math.max(0, Math.min(364, index + delta))
                              ]?.focus();
                            }}
                            onClick={() => setSelected(day)}
                            onFocus={() => setSelected(day)}
                          />
                        ) : (
                          <span
                            className='usage-calendar-pad'
                            key={`pad-${j}`}
                          />
                        ),
                      )}
                    </div>
                  ))}
                </div>
              </div>
              <div className='usage-calendar-footer'>
                <span aria-live='polite'>
                  {selected
                    ? `${selected.date} · ${number(selected.requests)} ${t('次')}`
                    : t('点击日期查看次数')}
                </span>
                <span className='usage-calendar-legend'>
                  {t('少')}
                  {[0, 1, 2, 3, 4].map((n) => (
                    <i key={n} className={`activity-${n}`} />
                  ))}
                  {t('多')}
                </span>
              </div>
              <div className='usage-calendar-meta'>
                <span>
                  {t('时区')}：{data.timezone} ·{' '}
                  {t('均值按 365 天计算，今天尚未结束；数据最多延迟 5 分钟。')}
                </span>
                <button type='button' onClick={load} aria-label={t('刷新')}>
                  <RotateCw size={14} />
                  {t('刷新')}
                </button>
              </div>
            </>
          )}
        </div>
      )}
    </section>
  );
}
