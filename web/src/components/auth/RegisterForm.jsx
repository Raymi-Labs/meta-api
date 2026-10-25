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
import { Link, useLocation, useNavigate } from 'react-router-dom';
import { API, updateAPI } from '../../helpers/apiCore';
import {
  onDiscordOAuthClicked,
  onCustomOAuthClicked,
  onGitHubOAuthClicked,
  onLinuxDOOAuthClicked,
  onOIDCClicked,
} from '../../helpers/api';
import { setUserData } from '../../helpers/data';
import { getLogo, getSystemName } from '../../helpers/storage';
import { showError, showInfo, showSuccess } from '../../helpers/toast';
import { Button, Divider, Form, Icon, Modal } from '@douyinfe/semi-ui';
import {
  IconGithubLogo,
  IconMail,
  IconUser,
  IconLock,
  IconKey,
} from '@douyinfe/semi-icons';
import OIDCIcon from '../common/logo/OIDCIcon';
import LinuxDoIcon from '../common/logo/LinuxDoIcon';
import WeChatIcon from '../common/logo/WeChatIcon';
import { UserContext } from '../../context/User';
import { StatusContext } from '../../context/Status';
import { useTranslation } from 'react-i18next';
import { SiDiscord } from 'react-icons/si';
import Turnstile from '../common/ResilientTurnstile';
import InviteBenefits from './InviteBenefits';
import AuthCard from './AuthCard';
import AuthConsent, { useAuthConsent } from './AuthConsent';
import {
  buildAuthPathWithNext,
  getSafeLoginRedirectPath,
  navigateToLoginRedirect,
} from '../../helpers/auth';

const TelegramLoginButton = lazy(() => import('react-telegram-login/src'));
const OAuthProviderIcon = lazy(() => import('../common/OAuthProviderIcon'));

const RegisterForm = () => {
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
    password2: '',
    email: '',
    verification_code: '',
    wechat_verification_code: '',
  });
  const { username, password, password2 } = inputs;
  const [, userDispatch] = useContext(UserContext);
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
  const [registerLoading, setRegisterLoading] = useState(false);
  const [verificationCodeLoading, setVerificationCodeLoading] = useState(false);
  const [wechatCodeSubmitLoading, setWechatCodeSubmitLoading] = useState(false);
  const [customOAuthLoading, setCustomOAuthLoading] = useState({});
  const [disableButton, setDisableButton] = useState(false);
  const [countdown, setCountdown] = useState(30);
  const [githubButtonState, setGithubButtonState] = useState('idle');
  const [githubButtonDisabled, setGithubButtonDisabled] = useState(false);
  const githubTimeoutRef = useRef(null);
  const turnstileWidgetRef = useRef(null);
  const githubButtonText = t(githubButtonTextKeyByState[githubButtonState]);

  const logo = getLogo();
  const systemName = getSystemName();
  const requestedPostLoginPath = getSafeLoginRedirectPath(location.search, '');
  const postLoginPath = requestedPostLoginPath || '/console';
  const loginPath = buildAuthPathWithNext('/login', requestedPostLoginPath);
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
  const hasOAuthRegisterOptions = Boolean(
    status.github_oauth ||
      status.discord_oauth ||
      status.oidc_enabled ||
      status.wechat_login ||
      status.linuxdo_oauth ||
      status.telegram_oauth ||
      hasCustomOAuthProviders,
  );

  const [showEmailVerification, setShowEmailVerification] = useState(false);
  const shouldCheckTurnstileForRegister = turnstileEnabled;

  useEffect(() => {
    setShowEmailVerification(!!status?.email_verification);
    if (status?.turnstile_check) {
      setTurnstileEnabled(true);
      setTurnstileSiteKey(status.turnstile_site_key);
    }
  }, [status]);

  useEffect(() => {
    let countdownInterval = null;
    if (disableButton && countdown > 0) {
      countdownInterval = setInterval(() => {
        setCountdown(countdown - 1);
      }, 1000);
    } else if (countdown === 0) {
      setDisableButton(false);
      setCountdown(30);
    }
    return () => clearInterval(countdownInterval); // Clean up on unmount
  }, [disableButton, countdown]);

  useEffect(() => {
    return () => {
      if (githubTimeoutRef.current) {
        clearTimeout(githubTimeoutRef.current);
      }
    };
  }, []);

  const resetTurnstileToken = () => {
    setTurnstileToken('');
    turnstileWidgetRef.current?.reset?.();
  };

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
      const params = new URLSearchParams({
        code: inputs.wechat_verification_code,
      });
      const savedAffCode = localStorage.getItem('aff');
      if (savedAffCode) {
        params.set('aff', savedAffCode);
      }
      const res = await API.get(`/api/oauth/wechat?${params.toString()}`);
      const { success, message, data } = res.data;
      if (success) {
        localStorage.removeItem('aff');
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

  async function handleSubmit() {
    if (registerLoading) return;
    if (!requireConsent()) return;
    if (password.length < 8) {
      showInfo(t('密码长度不得小于 8 位！'));
      return;
    }
    if (password !== password2) {
      showInfo(t('两次输入的密码不一致'));
      return;
    }
    if (username && password) {
      if (shouldCheckTurnstileForRegister && turnstileToken === '') {
        showInfo(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
        return;
      }
      setRegisterLoading(true);
      try {
        if (!affCode) {
          affCode = localStorage.getItem('aff');
        }
        inputs.aff_code = affCode;
        const turnstileQuery = shouldCheckTurnstileForRegister
          ? `?turnstile=${encodeURIComponent(turnstileToken)}`
          : '';
        const res = await API.post(
          `/api/user/register${turnstileQuery}`,
          inputs,
        );
        const { success, message } = res.data;
        if (success) {
          localStorage.removeItem('aff');
          navigate(loginPath);
          showSuccess(t('注册成功！'));
        } else {
          showError(message);
        }
      } catch (error) {
        showError(t('注册失败，请重试'));
      } finally {
        if (shouldCheckTurnstileForRegister) {
          resetTurnstileToken();
        }
        setRegisterLoading(false);
      }
    }
  }

  const sendVerificationCode = async () => {
    if (inputs.email === '') return;
    if (turnstileEnabled && turnstileToken === '') {
      showInfo(t('请稍后几秒重试，Turnstile 正在检查用户环境！'));
      return;
    }
    setVerificationCodeLoading(true);
    try {
      const res = await API.get(
        `/api/verification?email=${encodeURIComponent(
          inputs.email,
        )}&turnstile=${encodeURIComponent(turnstileToken)}`,
      );
      const { success, message } = res.data;
      if (success) {
        showSuccess(
          t('验证码发送成功，请检查邮箱（未收到请翻垃圾箱或广告邮件）'),
        );
        setDisableButton(true); // 发送成功后禁用按钮，开始倒计时
      } else {
        showError(message);
      }
    } catch (error) {
      showError(t('发送验证码失败，请重试'));
    } finally {
      if (turnstileEnabled) {
        resetTurnstileToken();
      }
      setVerificationCodeLoading(false);
    }
  };

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
      setTimeout(() => setGithubLoading(false), 3000);
    }
  };

  const handleDiscordClick = () => {
    if (!requireConsent()) return;
    setDiscordLoading(true);
    try {
      onDiscordOAuthClicked(status.discord_client_id, oauthLoginOptions);
    } finally {
      setTimeout(() => setDiscordLoading(false), 3000);
    }
  };

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
      setTimeout(() => setOidcLoading(false), 3000);
    }
  };

  const handleLinuxDOClick = () => {
    if (!requireConsent()) return;
    setLinuxdoLoading(true);
    try {
      onLinuxDOOAuthClicked(status.linuxdo_client_id, oauthLoginOptions);
    } finally {
      setTimeout(() => setLinuxdoLoading(false), 3000);
    }
  };

  const handleCustomOAuthClick = (provider) => {
    if (!requireConsent()) return;
    setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: true }));
    try {
      onCustomOAuthClicked(provider, oauthLoginOptions);
    } finally {
      setTimeout(() => {
        setCustomOAuthLoading((prev) => ({ ...prev, [provider.slug]: false }));
      }, 3000);
    }
  };

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
        localStorage.removeItem('aff');
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

  const renderOAuthOptions = () => (
    <div className='auth-providers'>
      {status.github_oauth && (
        <Button
          theme='outline'
          className={`auth-provider-button ${githubButtonState !== 'idle' ? 'auth-provider-button--wide' : ''}`}
          type='tertiary'
          icon={<IconGithubLogo size='large' />}
          aria-label={githubButtonText}
          onClick={handleGitHubClick}
          loading={githubLoading}
          disabled={githubButtonDisabled}
        >
          {githubButtonState === 'idle' ? 'GitHub' : githubButtonText}
        </Button>
      )}
      {(status.custom_oauth_providers || []).map((provider) => (
        <Button
          key={provider.slug}
          theme='outline'
          className='auth-provider-button'
          type='tertiary'
          icon={
            <Suspense fallback={null}>
              <OAuthProviderIcon iconName={provider.icon || ''} size={20} />
            </Suspense>
          }
          aria-label={t('使用 {{name}} 继续', { name: provider.name })}
          onClick={() => handleCustomOAuthClick(provider)}
          loading={customOAuthLoading[provider.slug]}
        >
          {provider.name}
        </Button>
      ))}
      {status.wechat_login && (
        <Button
          theme='outline'
          className='auth-provider-button'
          type='tertiary'
          icon={<Icon svg={<WeChatIcon />} style={{ color: '#07C160' }} />}
          onClick={onWeChatLoginClicked}
          loading={wechatLoading}
        >
          {t('使用 微信 继续')}
        </Button>
      )}
      {status.discord_oauth && (
        <Button
          theme='outline'
          className='auth-provider-button'
          type='tertiary'
          icon={
            <SiDiscord style={{ color: '#5865F2', width: 20, height: 20 }} />
          }
          aria-label={t('使用 Discord 继续')}
          onClick={handleDiscordClick}
          loading={discordLoading}
        >
          Discord
        </Button>
      )}
      {status.oidc_enabled && (
        <Button
          theme='outline'
          className='auth-provider-button'
          type='tertiary'
          icon={<OIDCIcon style={{ color: '#1877F2' }} />}
          aria-label={t('使用 OIDC 继续')}
          onClick={handleOIDCClick}
          loading={oidcLoading}
        >
          OIDC
        </Button>
      )}
      {status.linuxdo_oauth && (
        <Button
          theme='outline'
          className='auth-provider-button'
          type='tertiary'
          icon={
            <LinuxDoIcon style={{ color: '#E95420', width: 20, height: 20 }} />
          }
          aria-label={t('使用 LinuxDO 继续')}
          onClick={handleLinuxDOClick}
          loading={linuxdoLoading}
        >
          LinuxDO
        </Button>
      )}
      {status.telegram_oauth && (
        <div className='auth-provider-button--wide flex justify-center'>
          {hasConsent ? (
            <Suspense fallback={null}>
              <TelegramLoginButton
                dataOnauth={onTelegramLoginClicked}
                botName={status.telegram_bot_name}
              />
            </Suspense>
          ) : (
            <Button
              className='auth-provider-button'
              theme='outline'
              type='tertiary'
              disabled
            >
              Telegram
            </Button>
          )}
        </div>
      )}
    </div>
  );

  const renderRegisterForm = () => (
    <AuthCard
      logo={logo}
      systemName={systemName}
      title={t('创建账户')}
      footer={
        <>
          {t('已有账户？')} <Link to={loginPath}>{t('登录')}</Link>
        </>
      }
    >
      {hasOAuthRegisterOptions && (
        <>
          {renderOAuthOptions()}
          <Divider margin='24px' align='center'>
            {t('或')}
          </Divider>
        </>
      )}
      <Form className='auth-form' onSubmit={handleSubmit}>
        <Form.Input
          field='username'
          label={t('用户名')}
          placeholder={t('请输入用户名')}
          name='username'
          autoComplete='username'
          onChange={(value) => handleChange('username', value)}
          prefix={<IconUser />}
        />
        <Form.Input
          field='password'
          label={t('密码')}
          placeholder={t('输入密码，最短 8 位，最长 20 位')}
          name='password'
          autoComplete='new-password'
          mode='password'
          onChange={(value) => handleChange('password', value)}
          prefix={<IconLock />}
        />
        <Form.Input
          field='password2'
          label={t('确认密码')}
          placeholder={t('确认密码')}
          name='password2'
          autoComplete='new-password'
          mode='password'
          onChange={(value) => handleChange('password2', value)}
          prefix={<IconLock />}
        />
        {showEmailVerification && (
          <>
            <Form.Input
              field='email'
              label={t('邮箱')}
              placeholder={t('输入邮箱地址')}
              name='email'
              type='email'
              autoComplete='email'
              onChange={(value) => handleChange('email', value)}
              prefix={<IconMail />}
              suffix={
                <Button
                  htmlType='button'
                  onClick={sendVerificationCode}
                  loading={verificationCodeLoading}
                  disabled={disableButton || verificationCodeLoading}
                >
                  {disableButton
                    ? `${t('重新发送')} (${countdown})`
                    : t('获取验证码')}
                </Button>
              }
            />
            <Form.Input
              field='verification_code'
              label={t('验证码')}
              placeholder={t('输入验证码')}
              name='verification_code'
              autoComplete='one-time-code'
              onChange={(value) => handleChange('verification_code', value)}
              prefix={<IconKey />}
              extraText={
                disableButton ? (
                  <span>
                    {t(
                      '若未收到电子邮件，请到垃圾箱或广告邮件中查找；如仍未收到，请',
                    )}
                    <a
                      href='https://qm.qq.com/q/rAMnpMsFvq'
                      target='_blank'
                      rel='noopener noreferrer'
                      className='text-blue-500 hover:underline'
                    >
                      {t('联系客服人员')}
                    </a>
                  </span>
                ) : undefined
              }
            />
          </>
        )}
        <AuthConsent status={status} value={consent} onChange={setConsent} />
        {turnstileEnabled && (
          <div className='auth-captcha'>
            <Turnstile
              sitekey={turnstileSiteKey}
              onLoad={(_, boundTurnstile) => {
                turnstileWidgetRef.current = boundTurnstile;
              }}
              onVerify={(token, boundTurnstile) => {
                turnstileWidgetRef.current = boundTurnstile;
                setTurnstileToken(token);
              }}
              onExpire={() => setTurnstileToken('')}
              onError={() => setTurnstileToken('')}
              onTimeout={() => setTurnstileToken('')}
            />
          </div>
        )}
        <Button
          theme='solid'
          className='auth-submit'
          type='primary'
          htmlType='submit'
          loading={registerLoading}
          disabled={!hasConsent || registerLoading}
        >
          {t('注册')}
        </Button>
      </Form>
    </AuthCard>
  );

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

  return (
    <div className={`auth-page ${affCode ? 'invite-register-page' : ''}`}>
      <div
        className={affCode ? 'invite-register-layout' : 'auth-page__container'}
      >
        {affCode && <InviteBenefits />}
        <div id='invite-register-form' className='auth-page__container mx-auto'>
          {renderRegisterForm()}
          {renderWeChatLoginModal()}
        </div>
      </div>
    </div>
  );
};

export default RegisterForm;
