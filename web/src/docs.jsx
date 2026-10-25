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
import ReactDOM from 'react-dom/client';
import { BrowserRouter } from 'react-router-dom';
import { UserProvider } from './context/User';
import { ThemeProvider } from './context/Theme';
import { initialLocaleReady } from './i18n/i18n';
import DocsSite from './pages/Docs/Site';
import '@douyinfe/semi-ui/dist/css/semi.css';
import 'react-toastify/dist/ReactToastify.css';
import './index.css';

// The language store consumes the allowlisted locale from the entry URL.
// Never restore a main-site identity here.
initialLocaleReady.then(() =>
  ReactDOM.createRoot(document.getElementById('root')).render(
    <React.StrictMode>
      <UserProvider>
        <BrowserRouter>
          <ThemeProvider>
            <DocsSite />
          </ThemeProvider>
        </BrowserRouter>
      </UserProvider>
    </React.StrictMode>,
  ),
);
