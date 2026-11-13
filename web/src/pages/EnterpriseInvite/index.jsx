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

import React, { useEffect, useState } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { useTranslation } from 'react-i18next';
import { Banner, Button, Spin } from '@douyinfe/semi-ui';
import {
  ENTERPRISE_ROLE_LABEL,
  enterpriseRequest,
  formatEnterpriseDate,
} from '../../helpers/enterprise';
import { buildAuthPathWithNext } from '../../helpers/authRedirect';
import { getStoredUser } from '../../helpers/storage';
import { renderQuota } from '../../helpers/render';
import { refreshEnterpriseSelf } from '../../hooks/enterprise/useEnterpriseSelf';
import './enterpriseInvite.css';

export default function EnterpriseInvite() {
  const { t } = useTranslation();
  const location = useLocation();
  const navigate = useNavigate();
  const token = new URLSearchParams(location.search).get('token') || '';
  const user = getStoredUser();
  const [preview, setPreview] = useState(null);
  const [invalid, setInvalid] = useState('');
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);
  const [joined, setJoined] = useState(null);

  useEffect(() => {
    let alive = true;
    setPreview(null);
    setInvalid('');
    enterpriseRequest('get', '/api/enterprise/invitation', undefined, { token })
      .then((data) => alive && setPreview(data))
      .catch((e) => alive && setInvalid(e.message));
    return () => {
      alive = false;
    };
  }, [token]);

  const accept = async () => {
    setBusy(true);
    setError('');
    try {
      const result = await enterpriseRequest(
        'post',
        '/api/enterprise/invitation/accept',
        { token },
      );
      await refreshEnterpriseSelf();
      setJoined(result);
    } catch (e) {
      setError(e.message);
    } finally {
      setBusy(false);
    }
  };

  if (invalid) {
    return (
      <div className='ei'>
        <div className='ei-card'>
          <h1>{t('邀请已失效')}</h1>
          <p className='ei-note'>{invalid}</p>
          <Button theme='light' onClick={() => navigate('/')}>
            {t('返回首页')}
          </Button>
        </div>
      </div>
    );
  }

  if (!preview) {
    return (
      <div className='ei'>
        <Spin size='large' />
      </div>
    );
  }

  if (joined) {
    const manager = joined.role !== 'member';
    return (
      <div className='ei'>
        <div className='ei-card'>
          <span className='ei-lead'>{t('已加入')}</span>
          <h1>{joined.enterprise_name}</h1>
          <dl className='ei-facts'>
            <div>
              <dt>{t('可用额度')}</dt>
              <dd>{renderQuota(joined.available)}</dd>
            </div>
          </dl>
          {joined.quota_shortfall && (
            <Banner
              type='warning'
              fullMode={false}
              closeIcon={null}
              description={t('企业余额不足，初始额度暂未到账，已通知管理员。')}
            />
          )}
          <Button
            theme='solid'
            type='primary'
            size='large'
            block
            onClick={() =>
              navigate(manager ? '/enterprise/console' : '/console/enterprise')
            }
          >
            {manager ? t('进入企业工作台') : t('查看我的额度')}
          </Button>
        </div>
      </div>
    );
  }

  const back = `${location.pathname}${location.search}`;
  return (
    <div className='ei'>
      <div className='ei-card'>
        <span className='ei-lead'>{t('邀请你加入')}</span>
        <h1>{preview.enterprise_name}</h1>
        <dl className='ei-facts'>
          <div>
            <dt>{t('受邀邮箱')}</dt>
            <dd>{preview.email}</dd>
          </div>
          <div>
            <dt>{t('角色')}</dt>
            <dd>{t(ENTERPRISE_ROLE_LABEL[preview.role] || preview.role)}</dd>
          </div>
          <div>
            <dt>{t('有效期至')}</dt>
            <dd>{formatEnterpriseDate(preview.expires_at)}</dd>
          </div>
          {user && (
            <div>
              <dt>{t('当前账号')}</dt>
              <dd>{user.display_name || user.username}</dd>
            </div>
          )}
        </dl>
        {error && (
          <Banner
            type='danger'
            fullMode={false}
            closeIcon={null}
            description={
              // 只有邮箱不符时才需要去个人设置绑定邮箱，其他失败原因不带这个入口。
              error.includes('邮箱') ? (
                <>
                  {error} <Link to='/console/personal'>{t('个人设置')}</Link>
                </>
              ) : (
                error
              )
            }
          />
        )}
        {user ? (
          <Button
            theme='solid'
            type='primary'
            size='large'
            block
            loading={busy}
            onClick={accept}
          >
            {t('加入企业')}
          </Button>
        ) : (
          <div className='ei-actions'>
            <Button
              theme='solid'
              type='primary'
              size='large'
              block
              onClick={() => navigate(buildAuthPathWithNext('/login', back))}
            >
              {t('登录后加入')}
            </Button>
            <Button
              theme='light'
              size='large'
              block
              onClick={() => navigate(buildAuthPathWithNext('/register', back))}
            >
              {t('注册新账号')}
            </Button>
          </div>
        )}
      </div>
    </div>
  );
}
