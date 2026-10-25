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

import React, {
  lazy,
  Suspense,
  useContext,
  useEffect,
  useMemo,
  useRef,
  useState,
} from 'react';
import {
  Link,
  useLocation,
  useNavigate,
  useSearchParams,
} from 'react-router-dom';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { API, updateAPI } from '../../helpers/apiCore';
import {
  onGitHubOAuthClicked,
  onDiscordOAuthClicked,
  onOIDCClicked,
  onLinuxDOOAuthClicked,
  onCustomOAuthClicked,
} from '../../helpers/api';
import {
  buildAuthPathWithNext,
  getSafeLoginRedirectPath,
  navigateToLoginRedirect,
} from '../../helpers/auth';
import { setUserData } from '../../helpers/data';
import {
  prepareCredentialRequestOptions,
  buildAssertionResult,
  isPasskeySupported,
} from '../../helpers/passkey';
import { getLogo, getSystemName } from '../../helpers/storage';
import { showError, showInfo, showSuccess } from '../../helpers/toast';
import { Button, Divider, Form, Icon, Modal } from '@douyinfe/semi-ui';

import { IconGithubLogo, IconKey } from '@douyinfe/semi-icons';
import OIDCIcon from '../common/logo/OIDCIcon';
import WeChatIcon from '../common/logo/WeChatIcon';
import LinuxDoIcon from '../common/logo/LinuxDoIcon';
import TwoFAVerification from './TwoFAVerification';
import { useTranslation } from 'react-i18next';
import { SiDiscord } from 'react-icons/si';
import Turnstile from '../common/ResilientTurnstile';
import AuthCard from './AuthCard';
import AuthConsent, { useAuthConsent } from './AuthConsent';

const TelegramLoginButton = lazy(() => import('react-telegram-login'));
const OAuthProviderIcon = lazy(() => import('../common/OAuthProviderIcon'));

const LoginForm = () => {
  let navigate = useNavigate();
  const location = useLocation();
  const { t } = useTranslation();
  const githubButtonTextKeyByState = {
    idle: '使用 GitHub 继续',
    redirecting: '正在跳转 GitHub...',
    timeout: '请求超时，请刷新页面后重新发起 GitHub 登录',
  };
  const [inputs, setInputs] = useState({
    username: '',
    password: '',
    wechat_verification_code: '',
  });
  const { username, password } = inputs;
  const [searchParams, setSearchParams] = useSearchParams();
  const [submitted, setSubmitted] = useState(false);
  const [userState, userDispatch] = useContext(UserContext);
  const [statusState] = useContext(StatusContext);
  const [turnstileEnabled, setTurnstileEnabled] = useState(false);
  const [turnstileSiteKey, setTurnstileSiteKey] = useState('');
  const [turnstileToken, setTurnstileToken] = useState('');
  const [showWeChatLoginModal, setShowWeChatLoginModal] = useState(false);
  const [wechatLoading, setWechatLoading] = useState(false);
  const [githubLoading, setGithubLoading] = useState(false);
  const [discordLoading, setDiscordLoading] = useState(false);
  const [oidcLoading, setOidcLoading] = useState(false);
  const [linuxdoLoading, setLinuxdoLoading] = useState(false);
  const [loginLoading, setLoginLoading] = useState(false);
  const [resetPasswordLoading, setResetPasswordLoading] = useState(false);
  const [wechatCodeSubmitLoading, setWechatCodeSubmitLoading] = useState(false);
  const [showTwoFA, setShowTwoFA] = useState(false);
  const [passkeySupported, setPasskeySupported] = useState(false);
  const [passkeyLoading, setPasskeyLoading] = useState(false);
  const [githubButtonState, setGithubButtonState] = useState('idle');
  const [githubButtonDisabled, setGithubButtonDisabled] = useState(false);
  const githubTimeoutRef = useRef(null);
  const githubButtonText = t(githubButtonTextKeyByState[githubButtonState]);
  const [customOAuthLoading, setCustomOAuthLoading] = useState({});

  const logo = getLogo();
  const systemName = getSystemName();
  const requestedPostLoginPath = getSafeLoginRedirectPath(location.search, '');
  const postLoginPath = requestedPostLoginPath || '/console';
  const registerPath = buildAuthPathWithNext(
    '/register',
    requestedPostLoginPath,
  );
  const oauthLoginOptions = {
    shouldLogout: true,
    postLoginPath: requestedPostLoginPath,
  };
  const navigateToPostLogin = () =>
    navigateToLoginRedirect(postLoginPath, navigate);

  let affCode = new URLSearchParams(location.search).get('aff');
  if (affCode) {
    localStorage.setItem('aff', affCode);
  }

  const status = useMemo(() => {
    if (statusState?.status) return statusState.status;
    const savedStatus = localStorage.getItem('status');
    if (!savedStatus) return {};
    try {
      return JSON.parse(savedStatus) || {};
    } catch (err) {
      return {};
    }
  }, [statusState?.status]);
  const { consent, setConsent, hasConsent, requireConsent } = useAuthConsent(
    status,
    t,
  );
  const hasCustomOAuthProviders =
    (status.custom_oauth_providers || []).length > 0;
  const hasOAuthLoginOptions = Boolean(
    status.github_oauth ||
      status.discord_oauth ||
      status.oidc_enabled ||
      status.wechat_login ||
      status.linuxdo_oauth ||
      status.telegram_oauth ||
      hasCustomOAuthProviders,
  );

  useEffect(() => {
    if (status?.turnstile_check) {
      setTurnstileEnabled(true);
      setTurnstileSiteKey(status.turnstile_site_key);
    }
  }, [status]);

  useEffect(() => {
    isPasskeySupported()
      .then(setPasskeySupported)
      .catch(() => setPasskeySupported(false));

    return () => {
      if (githubTimeoutRef.current) {
        clearTimeout(githubTimeoutRef.current);
      }
    };
  }, []);

  useEffect(() => {
    if (searchParams.get('expired')) {
      showError(t('未登录或登录已过期，请重新登录'));
    }
  }, []);

  const onWeChatLoginClicked = () => {
    if (!requireConsent()) return;
    setWechatLoading(true);
    setShowWeChatLoginModal(true);
    setWechatLoading(false);
  };

  const onSubmitWeChatVerificationCode = async () => {
    if (!requireConsent()) return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    setWechatCodeSubmitLoading(true);
    try {
      const res = await API.get(
        `/api/oauth/wechat?code=${inputs.wechat_verification_code}`,
      );
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        setUserData(data);
        updateAPI();
        navigateToPostLogin();
        showSuccess(t('登录成功！'));
        setShowWeChatLoginModal(false);
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('登录失败，请重试'));
    } finally {
      setWechatCodeSubmitLoading(false);
    }
  };

  function handleChange(name, value) {
    setInputs((inputs) => ({ ...inputs, [name]: value }));
  }

  async function handleSubmit(e) {
    if (loginLoading) return;
    if (!requireConsent()) return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    setSubmitted(true);
    setLoginLoading(true);
    try {
      if (username && password) {
        const res = await API.post(
          `/api/user/login?turnstile=${turnstileToken}`,
          {
            username,
            password,
          },
        );
        const { success, message, data } = res.data;
        if (success) {
          // 检查是否需要2FA验证
          if (data && data.require_2fa) {
            setShowTwoFA(true);
            setLoginLoading(false);
            return;
          }

          userDispatch({ type: 'login', payload: data });
          setUserData(data);
          updateAPI();
          showSuccess(t('登录成功！'));
          if (username === 'root' && password === '123456') {
            Modal.error({
              title: t('您正在使用默认密码！'),
              content: t('请立刻修改默认密码！'),
              centered: true,
            });
          }
          navigateToPostLogin();
        } else {
          showError(message);
        }
      } else {
        showError(t('请输入用户名和密码！'));
      }
    } catch (error) {
      showError(t('登录失败，请重试'));
    } finally {
      setLoginLoading(false);
    }
  }

  // 添加Telegram登录处理函数
  const onTelegramLoginClicked = async (response) => {
    if (!requireConsent()) return;
    const fields = [
      'id',
      'first_name',
      'last_name',
      'username',
      'photo_url',
      'auth_date',
      'hash',
      'lang',
    ];
    const params = {};
    fields.forEach((field) => {
      if (response[field]) {
        params[field] = response[field];
      }
    });
    try {
      const res = await API.get(`/api/oauth/telegram/login`, { params });
      const { success, message, data } = res.data;
      if (success) {
        userDispatch({ type: 'login', payload: data });
        localStorage.setItem('user', JSON.stringify(data));
        showSuccess(t('登录成功！'));
        setUserData(data);
        updateAPI();
        navigateToPostLogin();
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('登录失败，请重试'));
    }
  };

  // 包装的GitHub登录点击处理
  const handleGitHubClick = () => {
    if (!requireConsent()) return;
    if (githubButtonDisabled) {
      return;
    }
    setGithubLoading(true);
    setGithubButtonDisabled(true);
    setGithubButtonState('redirecting');
    if (githubTimeoutRef.current) {
      clearTimeout(githubTimeoutRef.current);
    }
    githubTimeoutRef.current = setTimeout(() => {
      setGithubLoading(false);
      setGithubButtonState('timeout');
      setGithubButtonDisabled(true);
    }, 20000);
    try {
      onGitHubOAuthClicked(status.github_client_id, oauthLoginOptions);
    } finally {
      // 由于重定向，这里不会执行到，但为了完整性添加
      setTimeout(() => setGithubLoading(false), 3000);
    }
  };

  // 包装的Discord登录点击处理
  const handleDiscordClick = () => {
    if (!requireConsent()) return;
    setDiscordLoading(true);
    try {
      onDiscordOAuthClicked(status.discord_client_id, oauthLoginOptions);
    } finally {
      // 由于重定向，这里不会执行到，但为了完整性添加
      setTimeout(() => setDiscordLoading(false), 3000);
    }
  };

  // 包装的OIDC登录点击处理
  const handleOIDCClick = () => {
    if (!requireConsent()) return;
    setOidcLoading(true);
    try {
      onOIDCClicked(
        status.oidc_authorization_endpoint,
        status.oidc_client_id,
        false,
        oauthLoginOptions,
      );
    } finally {
      // 由于重定向，这里不会执行到，但为了完整性添加
      setTimeout(() => setOidcLoading(false), 3000);
    }
  };

  // 包装的LinuxDO登录点击处理
  const handleLinuxDOClick = () => {
    if (!requireConsent()) return;
    setLinuxdoLoading(true);
    try {
      onLinuxDOOAuthClicked(status.linuxdo_client_id, oauthLoginOptions);
    } finally {
      // 由于重定向，这里不会执行到，但为了完整性添加
      setTimeout(() => setLinuxdoLoading(false), 3000);
    }
  };

  // 包装的自定义OAuth登录点击处理
  const handleCustomOAuthClick = (provider) => {
    if (!requireConsent()) return;
    setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: true }));
    try {
      onCustomOAuthClicked(provider, oauthLoginOptions);
    } finally {
      // 由于重定向，这里不会执行到，但为了完整性添加
      setTimeout(() => {
        setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: false }));
      }, 3000);
    }
  };

  const handlePasskeyLogin = async () => {
    if (!requireConsent()) return;
    if (!passkeySupported) {
      showInfo(t('当前环境无法使用 Passkey 登录'));
      return;
    }
    if (!window.PublicKeyCredential) {
      showInfo(t('当前浏览器不支持 Passkey'));
      return;
    }

    setPasskeyLoading(true);
    try {
      const beginRes = await API.post('/api/user/passkey/login/begin');
      const { success, message, data } = beginRes.data;
      if (!success) {
        showError(message || t('无法发起 Passkey 登录'));
        return;
      }

      const publicKeyOptions = prepareCredentialRequestOptions(
        data?.options || data?.publicKey || data,
      );
      const assertion = await navigator.credentials.get({
        publicKey: publicKeyOptions,
      });
      const payload = buildAssertionResult(assertion);
      if (!payload) {
        showError(t('Passkey 验证失败，请重试'));
        return;
      }

      const finishRes = await API.post(
        '/api/user/passkey/login/finish',
        payload,
      );
      const finish = finishRes.data;
      if (finish.success) {
        userDispatch({ type: 'login', payload: finish.data });
        setUserData(finish.data);
        updateAPI();
        showSuccess(t('登录成功！'));
        navigateToPostLogin();
      } else {
        showError(finish.message || t('Passkey 登录失败，请重试'));
      }
    } catch (error) {
      if (error?.name === 'AbortError') {
        showInfo(t('已取消 Passkey 登录'));
      } else {
        showError(t('Passkey 登录失败，请重试'));
      }
    } finally {
      setPasskeyLoading(false);
    }
  };

  // 包装的重置密码点击处理
  const handleResetPasswordClick = () => {
    setResetPasswordLoading(true);
    navigate('/reset');
    setResetPasswordLoading(false);
  };

  // 2FA验证成功处理
  const handle2FASuccess = (data) => {
    userDispatch({ type: 'login', payload: data });
    setUserData(data);
    updateAPI();
    showSuccess(t('登录成功！'));
    navigateToPostLogin();
  };

  // 返回登录页面
  const handleBackToLogin = () => {
    setShowTwoFA(false);
    setInputs({ username: '', password: '', wechat_verification_code: '' });
  };

  const renderLoginForm = () => (
    <AuthCard
      logo={logo}
      systemName={systemName}
      title={t('登录')}
      footer={
        !status.self_use_mode_enabled && (
          <>
            {t('没有账户？')} <Link to={registerPath}>{t('注册')}</Link>
          </>
        )
      }
    >
      {(hasOAuthLoginOptions || (status.passkey_login && passkeySupported)) && (
        <>
          <div className='auth-providers'>
            {status.wechat_login && (
              <Button
                theme='outline'
                className='auth-provider-button'
                type='tertiary'
                icon={
                  <Icon svg={<WeChatIcon />} style={{ color: '#07C160' }} />
                }
                onClick={onWeChatLoginClicked}
                loading={wechatLoading}
              >
                <span>{t('微信')}</span>
              </Button>
            )}

            {status.github_oauth && (
              <Button
                theme='outline'
                className={`auth-provider-button ${githubButtonState !== 'idle' ? 'auth-provider-button--wide' : ''}`}
                aria-label={githubButtonText}
                type='tertiary'
                icon={<IconGithubLogo size='large' />}
                onClick={handleGitHubClick}
                loading={githubLoading}
                disabled={githubButtonDisabled}
              >
                <span>
                  {githubButtonState === 'idle' ? 'GitHub' : githubButtonText}
                </span>
              </Button>
            )}

            {status.discord_oauth && (
              <Button
                theme='outline'
                className='auth-provider-button'
                type='tertiary'
                icon={
                  <SiDiscord
                    style={{
                      color: '#5865F2',
                      width: '20px',
                      height: '20px',
                    }}
                  />
                }
                onClick={handleDiscordClick}
                loading={discordLoading}
              >
                <span>Discord</span>
              </Button>
            )}

            {status.oidc_enabled && (
              <Button
                theme='outline'
                className='auth-provider-button'
                type='tertiary'
                icon={<OIDCIcon style={{ color: '#1877F2' }} />}
                onClick={handleOIDCClick}
                loading={oidcLoading}
              >
                <span>OIDC</span>
              </Button>
            )}

            {status.linuxdo_oauth && (
              <Button
                theme='outline'
                className='auth-provider-button'
                type='tertiary'
                icon={
                  <LinuxDoIcon
                    style={{
                      color: '#E95420',
                      width: '20px',
                      height: '20px',
                    }}
                  />
                }
                onClick={handleLinuxDOClick}
                loading={linuxdoLoading}
              >
                <span>LinuxDO</span>
              </Button>
            )}

            {status.custom_oauth_providers &&
              status.custom_oauth_providers.map((provider) => (
                <Button
                  key={provider.slug}
                  theme='outline'
                  className='auth-provider-button'
                  type='tertiary'
                  icon={
                    <Suspense fallback={null}>
                      <OAuthProviderIcon
                        iconName={provider.icon || ''}
                        size={20}
                      />
                    </Suspense>
                  }
                  aria-label={t('使用 {{name}} 继续', { name: provider.name })}
                  onClick={() => handleCustomOAuthClick(provider)}
                  loading={customOAuthLoading[provider.slug]}
                >
                  <span>{provider.name}</span>
                </Button>
              ))}

            {status.telegram_oauth && (
              <div className='auth-providers__telegram flex justify-center'>
                {hasConsent ? (
                  <Suspense fallback={null}>
                    <TelegramLoginButton
                      dataOnauth={onTelegramLoginClicked}
                      botName={status.telegram_bot_name}
                    />
                  </Suspense>
                ) : (
                  <Button disabled className='auth-provider-button'>
                    Telegram
                  </Button>
                )}
              </div>
            )}

            {status.passkey_login && passkeySupported && (
              <Button
                theme='outline'
                className='auth-provider-button'
                type='tertiary'
                icon={<IconKey size='large' />}
                onClick={handlePasskeyLogin}
                loading={passkeyLoading}
              >
                <span>{t('使用 Passkey 登录')}</span>
              </Button>
            )}
          </div>
          <Divider margin='22px' align='center'>
            {t('或')}
          </Divider>
        </>
      )}
      <Form onSubmit={handleSubmit}>
        <Form.Input
          field='username'
          label={t('用户名或邮箱')}
          placeholder={t('请输入您的用户名或邮箱地址')}
          name='username'
          autoComplete='username'
          onChange={(value) => handleChange('username', value)}
        />
        <Form.Input
          field='password'
          label={t('密码')}
          placeholder={t('请输入您的密码')}
          name='password'
          mode='password'
          autoComplete='current-password'
          onChange={(value) => handleChange('password', value)}
        />
        <AuthConsent status={status} value={consent} onChange={setConsent} />
        {turnstileEnabled && (
          <div className='auth-captcha'>
            <Turnstile
              sitekey={turnstileSiteKey}
              onVerify={setTurnstileToken}
              onExpire={() => setTurnstileToken('')}
              onError={() => setTurnstileToken('')}
              onTimeout={() => setTurnstileToken('')}
            />
          </div>
        )}
        <Button
          theme='solid'
          type='primary'
          htmlType='submit'
          className='auth-submit'
          block
          loading={loginLoading}
          disabled={!hasConsent || loginLoading}
        >
          {t('继续')}
        </Button>
        <Button
          theme='borderless'
          type='tertiary'
          block
          className='mt-3'
          onClick={handleResetPasswordClick}
          loading={resetPasswordLoading}
        >
          {t('忘记密码？')}
        </Button>
      </Form>
    </AuthCard>
  );

  // 微信登录模态框
  const renderWeChatLoginModal = () => {
    return (
      <Modal
        title={t('微信扫码登录')}
        visible={showWeChatLoginModal}
        maskClosable={true}
        onOk={onSubmitWeChatVerificationCode}
        onCancel={() => setShowWeChatLoginModal(false)}
        okText={t('登录')}
        centered={true}
        okButtonProps={{
          loading: wechatCodeSubmitLoading,
        }}
      >
        <div className='flex flex-col items-center'>
          <img
            src={status.wechat_qrcode}
            alt={t('微信二维码')}
            className='mb-4'
          />
        </div>

        <div className='text-center mb-4'>
          <p>
            {t('微信扫码关注公众号，输入「验证码」获取验证码（三分钟内有效）')}
          </p>
        </div>

        <Form>
          <Form.Input
            field='wechat_verification_code'
            placeholder={t('验证码')}
            label={t('验证码')}
            value={inputs.wechat_verification_code}
            onChange={(value) =>
              handleChange('wechat_verification_code', value)
            }
          />
        </Form>
      </Modal>
    );
  };

  // 2FA验证弹窗
  const render2FAModal = () => {
    return (
      <Modal
        title={
          <div className='flex items-center'>
            <div className='w-8 h-8 rounded-full bg-green-100 dark:bg-green-900 flex items-center justify-center mr-3'>
              <svg
                className='w-4 h-4 text-green-600 dark:text-green-400'
                fill='currentColor'
                viewBox='0 0 20 20'
              >
                <path
                  fillRule='evenodd'
                  d='M6 8a2 2 0 11-4 0 2 2 0 014 0zM8 7a1 1 0 100 2h8a1 1 0 100-2H8zM6 14a2 2 0 11-4 0 2 2 0 014 0zM8 13a1 1 0 100 2h8a1 1 0 100-2H8z'
                  clipRule='evenodd'
                />
              </svg>
            </div>
            {t('两步验证')}
          </div>
        }
        visible={showTwoFA}
        onCancel={handleBackToLogin}
        footer={null}
        width={450}
        centered
      >
        <TwoFAVerification
          onSuccess={handle2FASuccess}
          onBack={handleBackToLogin}
          isModal={true}
        />
      </Modal>
    );
  };

  return (
    <div className='auth-page'>
      <div className='auth-page__container'>
        {renderLoginForm()}
        {renderWeChatLoginModal()}
        {render2FAModal()}
      </div>
    </div>
  );
};

export default LoginForm;
