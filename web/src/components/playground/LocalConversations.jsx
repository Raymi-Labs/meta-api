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
import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { getPlaygroundOwner, loadConfig, loadMessages } from './configStorage';
import { openSessionStore } from './sessionStorage';
import {
  BACKUP_LIMIT,
  SESSION_LIMIT,
  createConversationGate,
  newPlaygroundSession,
  recoverSession,
  parseSessionBackup,
  sessionBackup,
  sessionTitle,
} from '../../helpers/playgroundSessions';
import './localConversations.css';
import {
  Plus,
  MessagesSquare,
  SlidersHorizontal,
  Code2,
  X,
} from 'lucide-react';
import './playgroundFocused.css';
import { useIsMobile } from '../../hooks/common/useIsMobile';

export default function LocalConversations({ owner, renderConversation }) {
  const { t } = useTranslation();
  const isMobile = useIsMobile();
  const [sessions, setSessions] = useState([]);
  const [active, setActive] = useState(null);
  const [busy, setBusy] = useState(null);
  const [ready, setReady] = useState(false);
  const [error, setError] = useState('');
  const [saving, setSaving] = useState(false);
  const [filter, setFilter] = useState('');
  const [drawer, setDrawer] = useState(false);
  const [panel, setPanel] = useState(null);
  const historyButton = useRef(null),
    historySearch = useRef(null);
  const closeHistory = useCallback(() => {
    setDrawer(false);
    historyButton.current?.focus();
  }, []);
  useEffect(() => {
    if (!drawer) return;
    historySearch.current?.focus();
    const escape = (e) => {
      if (e.key === 'Escape') {
        e.preventDefault();
        closeHistory();
      }
    };
    window.addEventListener('keydown', escape);
    return () => window.removeEventListener('keydown', escape);
  }, [drawer, closeHistory]);
  const openPanel = (value) => {
    setDrawer(false);
    setPanel((previous) => (previous === value ? null : value));
  };
  const records = useRef([]),
    current = useRef(null),
    store = useRef(null),
    live = useRef(false);
  const timer = useRef(null),
    revision = useRef(0),
    gate = useRef(createConversationGate());
  const savedRevision = useRef(0);
  const fileInput = useRef(null);
  const valid = () => live.current && getPlaygroundOwner() === owner;
  const persist = useCallback(() => {
    clearTimeout(timer.current);
    timer.current = null;
    if (!store.current || !live.current || getPlaygroundOwner() !== owner)
      return;
    const rev = ++revision.current;
    setSaving(true);
    store.current
      .write({ version: 1, active: current.current, sessions: records.current })
      .then(() => {
        savedRevision.current = rev;
        if (live.current && rev === revision.current) {
          setSaving(false);
          setError('');
        }
      })
      .catch(() => {
        if (live.current) {
          setSaving(false);
          setError('本地保存失败，请导出备份后重试');
        }
      });
  }, [owner]);
  const schedule = useCallback(() => {
    revision.current++;
    setSaving(true);
    if (!timer.current) timer.current = setTimeout(persist, 250);
  }, [persist]);

  useEffect(() => {
    live.current = true;
    let disposed = false,
      unlock;
    let connection;
    const flush = () => persist();
    const beforeUnload = (e) => {
      const pending =
        gate.current.current || revision.current !== savedRevision.current;
      persist();
      if (pending) {
        e.preventDefault();
        e.returnValue = '';
      }
    };
    const start = async () => {
      if (!navigator.locks) {
        setError('此浏览器不支持安全的多标签页保护，请更新浏览器');
        return;
      }
      await navigator.locks.request(
        `meta-api-playground-owner-${owner}`,
        { ifAvailable: true },
        async (lock) => {
          if (disposed) return;
          if (!lock) {
            setError('此账号已在其他标签页打开模型体验，请关闭该页后重试');
            return;
          }
          try {
            connection = await openSessionStore(owner);
            if (disposed) {
              connection.close();
              return;
            }
            store.current = connection;
            const saved = await connection.read();
            if (disposed || getPlaygroundOwner() !== owner) return;
            let rows;
            if (saved) {
              if (
                saved.version !== 1 ||
                !Array.isArray(saved.sessions) ||
                saved.sessions.length > SESSION_LIMIT ||
                saved.sessions.some(
                  (s) => !s.id || !Array.isArray(s.messages) || !s.config,
                )
              )
                throw new Error('invalid storage');
              rows = saved.sessions.map(recoverSession);
            } else {
              const session = newPlaygroundSession();
              session.config = loadConfig(owner);
              session.messages = loadMessages(owner) || [];
              rows = [recoverSession(session)];
            }
            if (!rows.length) rows = [newPlaygroundSession()];
            records.current = rows;
            current.current = rows.some((s) => s.id === saved?.active)
              ? saved.active
              : rows[0].id;
            await connection.write({
              version: 1,
              active: current.current,
              sessions: rows,
            });
            if (disposed) return;
            setSessions(rows);
            setActive(current.current);
            setReady(true);
            await new Promise((resolve) => {
              unlock = resolve;
              if (disposed) resolve();
            });
          } catch {
            if (!disposed)
              setError('本地对话无法读取，请检查浏览器存储权限后重试');
          } finally {
            connection?.close();
          }
        },
      );
    };
    start().catch(() => {
      if (!disposed) setError('本地对话无法读取，请检查浏览器存储权限后重试');
    });
    window.addEventListener('pagehide', flush);
    window.addEventListener('beforeunload', beforeUnload);
    return () => {
      persist();
      disposed = true;
      live.current = false;
      clearTimeout(timer.current);
      timer.current = null;
      window.removeEventListener('pagehide', flush);
      window.removeEventListener('beforeunload', beforeUnload);
      // Release the tab lock only after pending saves settle.
      if (connection) connection.close().finally(() => unlock?.());
      else unlock?.();
      store.current = null;
    };
  }, [owner, persist]);

  const snapshot = useCallback(
    (id, patch) => {
      if (!live.current || getPlaygroundOwner() !== owner) return;
      const old = records.current.find((s) => s.id === id);
      if (!old) return;
      records.current = records.current.map((s) =>
        s.id === id ? { ...s, ...patch, updatedAt: Date.now() } : s,
      );
      // Keep stream payloads off the sidebar React state; thumbnails update on save/switch.
      schedule();
    },
    [owner, schedule],
  );
  const acquire = useCallback(
    (id) => {
      if (
        !live.current ||
        getPlaygroundOwner() !== owner ||
        !gate.current.acquire(id)
      )
        return false;
      setBusy(id);
      return true;
    },
    [owner],
  );
  const release = useCallback(
    (id) => {
      if (gate.current.current !== id) return;
      gate.current.release(id);
      setBusy(null);
      setSessions([...records.current]);
      persist();
    },
    [persist],
  );
  const select = (id) => {
    if (!valid()) return;
    current.current = id;
    setSessions([...records.current]);
    setActive(id);
    setDrawer(false);
    setPanel(null);
    persist();
  };
  const add = () => {
    if (!valid()) return;
    if (records.current.length >= SESSION_LIMIT) {
      setError('本地对话已达100个，请先导出并删除部分对话');
      return;
    }
    const config = records.current.find(
      (s) => s.id === current.current,
    )?.config;
    const session = newPlaygroundSession(config);
    records.current = [session, ...records.current];
    select(session.id);
  };
  const rename = (session) => {
    const title = window.prompt(
      t('重命名对话'),
      sessionTitle(session, t('新对话')),
    );
    if (title === null || !valid()) return;
    records.current = records.current.map((s) =>
      s.id === session.id ? { ...s, title: title.trim().slice(0, 80) } : s,
    );
    setSessions([...records.current]);
    persist();
  };
  const remove = (id) => {
    if (
      !valid() ||
      gate.current.current === id ||
      !window.confirm(t('删除此本地对话？此操作无法撤销，建议先导出备份。'))
    )
      return;
    records.current = records.current.filter((s) => s.id !== id);
    if (!records.current.length) records.current = [newPlaygroundSession()];
    select(current.current === id ? records.current[0].id : current.current);
  };
  const exportAll = () => {
    if (!valid()) return;
    const url = URL.createObjectURL(
      new Blob([sessionBackup(records.current)], { type: 'application/json' }),
    );
    const link = document.createElement('a');
    link.href = url;
    link.download = 'model-conversations.json';
    link.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };
  const importAll = async (event) => {
    const file = event.target.files?.[0];
    event.target.value = '';
    if (!file || !valid()) return;
    try {
      if (file.size > BACKUP_LIMIT) throw new Error('对话备份过大');
      const incoming = parseSessionBackup(await file.text());
      if (!valid()) return;
      if (records.current.length + incoming.length > SESSION_LIMIT)
        throw new Error('本地对话已达100个，请先导出并删除部分对话');
      records.current = [...incoming, ...records.current];
      select(incoming[0].id);
    } catch (e) {
      if (valid())
        setError(
          e.message === '对话备份过大'
            ? e.message
            : '对话备份格式无效或超过数量限制',
        );
    }
  };
  if (!ready)
    return (
      <div className='pg-local-state' role='status'>
        {error ? t(error) : t('正在读取本地对话…')}
        {error && (
          <button onClick={() => window.location.reload()}>{t('重试')}</button>
        )}
      </div>
    );
  const mounted = records.current.filter(
    (s) => s.id === active || s.id === busy,
  );
  return (
    <div className='pg-conversations pg-focused'>
      <nav className='pg-tool-rail' aria-label={t('模型体验工具')}>
        <button
          className='pg-rail-new'
          onClick={add}
          aria-label={t('新建对话')}
          title={t('新建对话')}
        >
          <Plus size={21} />
          <span>{t('新建')}</span>
        </button>
        <span className='pg-rail-divider' />
        <button
          ref={historyButton}
          onClick={() => {
            setPanel(null);
            setDrawer(!drawer);
          }}
          aria-expanded={drawer}
          aria-controls='pg-local-history'
          aria-label={t('对话列表')}
        >
          <MessagesSquare size={20} />
          <span>{t('对话')}</span>
        </button>
        <button
          onClick={() => openPanel('settings')}
          aria-expanded={panel === 'settings'}
          aria-label={t('对话设置')}
          title={
            records.current
              .find((s) => s.id === active)
              ?.config?.inputs?.systemPrompt?.trim()
              ? t('已设置系统提示词')
              : t('对话设置')
          }
        >
          <SlidersHorizontal size={20} />
          <span>{t('设置')}</span>
          {records.current
            .find((s) => s.id === active)
            ?.config?.inputs?.systemPrompt?.trim() && (
            <span
              className='pg-prompt-dot'
              role='img'
              aria-label={t('已设置系统提示词')}
            />
          )}
        </button>
        <button
          className='pg-rail-code'
          onClick={() => openPanel('code')}
          aria-expanded={panel === 'code'}
          aria-label={t('请求代码')}
        >
          <Code2 size={21} />
          <span>{t('代码')}</span>
        </button>
      </nav>
      <div className='pg-conversation-toolbar'>
        <span role='status'>
          {saving ? t('正在保存到本地…') : t('已保存在此浏览器')}
        </span>
      </div>
      {error && (
        <div role='alert' className='pg-local-warning'>
          {t(error)} <button onClick={persist}>{t('重试保存')}</button>
          <button onClick={exportAll}>{t('备份全部对话')}</button>
        </div>
      )}
      {busy && busy !== active && (
        <div role='status' className='pg-local-warning'>
          {t('另一个对话正在生成，请等待完成或返回该对话停止生成。')}{' '}
          <button onClick={() => select(busy)}>{t('返回生成中的对话')}</button>
        </div>
      )}
      <div className='pg-conversation-layout'>
        {drawer && isMobile && (
          <button
            className='pg-history-backdrop'
            onClick={closeHistory}
            aria-label={t('关闭对话列表')}
          />
        )}
        <aside
          id='pg-local-history'
          hidden={!drawer}
          className={`pg-conversation-list ${drawer ? 'is-open' : ''}`}
          aria-label={t('本地对话')}
        >
          <div className='pg-history-heading'>
            <strong>{t('本地对话')}</strong>
            <button onClick={closeHistory} aria-label={t('关闭对话列表')}>
              <X size={18} />
            </button>
          </div>
          <input
            ref={historySearch}
            aria-label={t('搜索对话标题')}
            placeholder={t('搜索对话标题')}
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
          />
          <div className='pg-conversation-items'>
            {sessions
              .filter((s) =>
                sessionTitle(s, t('新对话'))
                  .toLowerCase()
                  .includes(filter.toLowerCase()),
              )
              .map((s) => (
                <div
                  key={s.id}
                  className={s.id === active ? 'is-selected' : ''}
                >
                  <button
                    className='pg-conversation-select'
                    onClick={() => select(s.id)}
                    aria-current={s.id === active ? 'true' : undefined}
                  >
                    {sessionTitle(s, t('新对话'))}
                    {busy === s.id ? ` · ${t('生成中')}` : ''}
                  </button>
                  <button
                    onClick={() => rename(s)}
                    aria-label={t('重命名对话')}
                  >
                    ✎
                  </button>
                  <button
                    disabled={busy === s.id}
                    onClick={() => remove(s.id)}
                    aria-label={t('删除对话')}
                  >
                    ×
                  </button>
                </div>
              ))}
          </div>
          <div className='pg-conversation-backup'>
            <button onClick={exportAll}>{t('备份全部对话')}</button>
            <button onClick={() => fileInput.current.click()}>
              {t('导入对话备份')}
            </button>
          </div>
          <input
            ref={fileInput}
            type='file'
            accept='.json,application/json'
            hidden
            onChange={importAll}
          />
          <small>
            {t(
              '仅保存在当前浏览器，换设备不同步；清理网站数据可能丢失。备份包含聊天内容，请妥善保管。',
            )}
          </small>
        </aside>
        <div className='pg-conversation-content'>
          {mounted.map((s) => (
            <div key={s.id} hidden={s.id !== active}>
              {renderConversation({
                owner,
                session: s,
                onSnapshot: snapshot,
                acquire,
                release,
                blocked: !!busy && busy !== s.id,
                active: s.id === active,
                panel,
                setPanel,
              })}
            </div>
          ))}
        </div>
      </div>
    </div>
  );
}
