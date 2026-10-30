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

import React from 'react';
import { Navigate, useLocation } from 'react-router-dom';
import { history } from './history';
import {
  getSafeLoginRedirectPath,
  isBackendLoginRedirectPath,
} from './authRedirect';

export * from './authRedirect';

export function authHeader() {
  // return authorization header with jwt token
  let user = JSON.parse(localStorage.getItem('user'));

  if (user && user.token) {
    return { Authorization: 'Bearer ' + user.token };
  } else {
    return {};
  }
}

export const AuthRedirect = ({ children }) => {
  const { search } = useLocation();
  const user = localStorage.getItem('user');

  if (user) {
    // Browser history updates before a concurrent router transition commits.
    // Keep this guard on the same location snapshot as the rendered login page.
    const path = getSafeLoginRedirectPath(search);
    // A stale browser-side user record does not prove the server session used
    // by the SSO bridge is alive. Keep the login form visible after the bridge
    // redirects here instead of looping between /login and the backend route.
    if (isBackendLoginRedirectPath(path)) return children;
    return <Navigate to={path} replace />;
  }

  return children;
};

function PrivateRoute({ children, loginPath = '/login' }) {
  if (!localStorage.getItem('user')) {
    return <Navigate to={loginPath} state={{ from: history.location }} />;
  }
  return children;
}

export function AdminRoute({ children }) {
  const raw = localStorage.getItem('user');
  if (!raw) {
    return <Navigate to='/login' state={{ from: history.location }} />;
  }
  try {
    const user = JSON.parse(raw);
    if (user && typeof user.role === 'number' && user.role >= 10) {
      return children;
    }
  } catch (e) {
    // ignore
  }
  return <Navigate to='/forbidden' replace />;
}

export { PrivateRoute };
