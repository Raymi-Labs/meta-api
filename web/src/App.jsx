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

import React, { lazy, Suspense, useContext, useEffect, useMemo } from 'react';
import {
  Navigate,
  Route,
  Routes,
  useLocation,
  useParams,
} from 'react-router-dom';
import Loading from './components/common/ui/Loading';
import {
  AuthRedirect,
  PrivateRoute,
  AdminRoute,
  buildAuthPathWithNext,
} from './helpers/auth';
import NotFound from './pages/NotFound';
import Forbidden from './pages/Forbidden';
import { StatusContext } from './context/Status';
import { MODEL_DETAIL_ROUTE } from './helpers/modelDetailNavigation';

import SetupCheck from './components/layout/SetupCheck';

const RegisterForm = lazy(() => import('./components/auth/RegisterForm'));
const LoginForm = lazy(() => import('./components/auth/LoginForm'));
const PasswordResetForm = lazy(
  () => import('./components/auth/PasswordResetForm'),
);
const PasswordResetConfirm = lazy(
  () => import('./components/auth/PasswordResetConfirm'),
);
const OAuth2Callback = lazy(() => import('./components/auth/OAuth2Callback'));
const Home = lazy(() => import('./pages/Home'));
const Dashboard = lazy(() => import('./pages/Dashboard'));
const About = lazy(() => import('./pages/About'));
const User = lazy(() => import('./pages/User'));
const Setting = lazy(() => import('./pages/Setting'));
const Channel = lazy(() => import('./pages/Channel'));
const UpstreamAccounts = lazy(() => import('./pages/UpstreamAccounts'));
const Token = lazy(() => import('./pages/Token'));
const Redemption = lazy(() => import('./pages/Redemption'));
const TopUp = lazy(() => import('./pages/TopUp'));
const Log = lazy(() => import('./pages/Log'));
const Pulse = lazy(() => import('./pages/Pulse'));
const PulseOps = lazy(() => import('./pages/PulseOps'));
const DesktopFeedback = lazy(() => import('./pages/DesktopFeedback'));
const Chat = lazy(() => import('./pages/Chat'));
const Chat2Link = lazy(() => import('./pages/Chat2Link'));
const Midjourney = lazy(() => import('./pages/Midjourney'));
const Pricing = lazy(() => import('./pages/Pricing'));
const ModelDetail = lazy(() => import('./pages/ModelDetail'));
const Task = lazy(() => import('./pages/Task'));
const ModelPage = lazy(() => import('./pages/Model'));
const ModelDeploymentPage = lazy(() => import('./pages/ModelDeployment'));
const Playground = lazy(() => import('./pages/Playground'));
const ImagePlayground = lazy(() => import('./pages/ImagePlayground'));
const Subscription = lazy(() => import('./pages/Subscription'));
const Usage = lazy(() => import('./pages/Usage'));
const Docs = lazy(() => import('./pages/Docs'));
const UserAgreement = lazy(() => import('./pages/UserAgreement'));
const PrivacyPolicy = lazy(() => import('./pages/PrivacyPolicy'));
const PersonalSetting = lazy(
  () => import('./components/settings/PersonalSetting'),
);
const Setup = lazy(() => import('./pages/Setup'));

function DynamicOAuth2Callback() {
  const { provider } = useParams();
  return <OAuth2Callback type={provider} />;
}

function safeDecodePathname(pathname) {
  try {
    return decodeURIComponent(pathname);
  } catch (error) {
    return pathname;
  }
}

function getCanonicalPathFromFullwidthQuestion(locationLike) {
  const decodedPathname = safeDecodePathname(locationLike.pathname || '');
  const markerIndex = decodedPathname.indexOf('？');
  if (markerIndex < 0) {
    return '';
  }

  const basePath = decodedPathname.slice(0, markerIndex).replace(/\/+$/, '');
  const embeddedQuery = decodedPathname
    .slice(markerIndex + 1)
    .replace(/^[?&]+/, '');
  const params = new URLSearchParams(embeddedQuery);
  new URLSearchParams(locationLike.search || '').forEach((value, key) => {
    params.append(key, value);
  });
  const search = params.toString();
  return `${basePath || '/'}${search ? `?${search}` : ''}${locationLike.hash || ''}`;
}

function App() {
  const location = useLocation();
  const [statusState] = useContext(StatusContext);

  useEffect(() => {
    const canonicalPath = getCanonicalPathFromFullwidthQuestion(
      window.location,
    );
    if (canonicalPath) {
      window.location.replace(canonicalPath);
    }
  }, []);

  // 获取模型广场权限配置
  const pricingRequireAuth = useMemo(() => {
    const headerNavModulesConfig = statusState?.status?.HeaderNavModules;
    if (headerNavModulesConfig) {
      try {
        const modules = JSON.parse(headerNavModulesConfig);

        // 处理向后兼容性：如果pricing是boolean，默认不需要登录
        if (typeof modules.pricing === 'boolean') {
          return false; // 默认不需要登录鉴权
        }

        // 如果是对象格式，使用requireAuth配置
        return modules.pricing?.requireAuth === true;
      } catch (error) {
        console.error('解析顶栏模块配置失败:', error);
        return false; // 默认不需要登录
      }
    }
    return false; // 默认不需要登录
  }, [statusState?.status?.HeaderNavModules]);

  return (
    <SetupCheck>
      <Routes>
        <Route
          path='/'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Home />
            </Suspense>
          }
        />
        <Route
          path='/setup'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Setup />
            </Suspense>
          }
        />
        <Route path='/forbidden' element={<Forbidden />} />
        <Route
          path='/console/models'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <ModelPage />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/deployment'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <ModelDeploymentPage />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/subscription'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Subscription />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/upstream-accounts'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <UpstreamAccounts />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/profit-board'
          element={
            <AdminRoute>
              <Navigate to='/console/upstream-accounts' replace />
            </AdminRoute>
          }
        />
        <Route
          path='/console/channel'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Channel />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/token'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Token />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/playground'
          element={
            <Navigate
              to={`/playground${location.search}${location.hash}`}
              replace
            />
          }
        />
        <Route
          path='/playground'
          element={
            <PrivateRoute
              loginPath={buildAuthPathWithNext(
                '/login',
                `${location.pathname}${location.search}${location.hash}`,
              )}
            >
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Playground />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/image-playground'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <ImagePlayground />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/redemption'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Redemption />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/user'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <User />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/user/reset'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PasswordResetConfirm />
            </Suspense>
          }
        />
        <Route
          path='/login'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <AuthRedirect>
                <LoginForm />
              </AuthRedirect>
            </Suspense>
          }
        />
        <Route
          path='/register'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <AuthRedirect>
                <RegisterForm />
              </AuthRedirect>
            </Suspense>
          }
        />
        <Route
          path='/reset'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PasswordResetForm />
            </Suspense>
          }
        />
        <Route
          path='/oauth/github'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='github'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/discord'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='discord'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/oidc'
          element={
            <Suspense fallback={<Loading></Loading>}>
              <OAuth2Callback type='oidc'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/linuxdo'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <OAuth2Callback type='linuxdo'></OAuth2Callback>
            </Suspense>
          }
        />
        <Route
          path='/oauth/:provider'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <DynamicOAuth2Callback />
            </Suspense>
          }
        />
        <Route
          path='/console/setting'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Setting />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/personal'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PersonalSetting />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/topup'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <TopUp />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/pulse'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Pulse />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/desktop-feedback'
          element={
            <AdminRoute>
              <Suspense fallback={<Loading />} key={location.pathname}>
                <DesktopFeedback />
              </Suspense>
            </AdminRoute>
          }
        />
        <Route
          path='/console/pulse-ops'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <PulseOps />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/log'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Log />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Dashboard />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/midjourney'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Midjourney />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route
          path='/console/task'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Task />
              </Suspense>
            </PrivateRoute>
          }
        />
        {[
          { path: '/pricing', Component: Pricing },
          { path: MODEL_DETAIL_ROUTE, Component: ModelDetail },
        ].map(({ path, Component }) => (
          <Route
            key={path}
            path={path}
            element={
              pricingRequireAuth ? (
                <PrivateRoute>
                  <Suspense
                    fallback={<Loading></Loading>}
                    key={location.pathname}
                  >
                    <Component />
                  </Suspense>
                </PrivateRoute>
              ) : (
                <Suspense
                  fallback={<Loading></Loading>}
                  key={location.pathname}
                >
                  <Component />
                </Suspense>
              )
            }
          />
        ))}
        <Route
          path='/usage'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Usage />
            </Suspense>
          }
        />
        <Route
          path='/docs/*'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Docs />
            </Suspense>
          }
        />
        <Route
          path='/about'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <About />
            </Suspense>
          }
        />
        <Route
          path='/user-agreement'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <UserAgreement />
            </Suspense>
          }
        />
        <Route
          path='/privacy-policy'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <PrivacyPolicy />
            </Suspense>
          }
        />
        <Route
          path='/console/chat/:id?'
          element={
            <Suspense fallback={<Loading></Loading>} key={location.pathname}>
              <Chat />
            </Suspense>
          }
        />
        {/* 方便使用chat2link直接跳转聊天... */}
        <Route
          path='/chat2link'
          element={
            <PrivateRoute>
              <Suspense fallback={<Loading></Loading>} key={location.pathname}>
                <Chat2Link />
              </Suspense>
            </PrivateRoute>
          }
        />
        <Route path='*' element={<NotFound />} />
      </Routes>
    </SetupCheck>
  );
}

export default App;
