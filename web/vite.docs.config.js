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

import { defineConfig } from 'vite';
import {
  copyFileSync,
  cpSync,
  renameSync,
  readFileSync,
  writeFileSync,
} from 'node:fs';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import base from './vite.config';

export default defineConfig({
  ...base,
  publicDir: false,
  build: {
    ...base.build,
    target: 'esnext',
    outDir: 'docs-dist',
    rollupOptions: {
      ...base.build.rollupOptions,
      input: path.resolve('docs.html'),
    },
  },
  plugins: [
    ...base.plugins,
    {
      name: 'docs-artifacts',
      apply: 'build',
      closeBundle() {
        const output = path.resolve('docs-dist');
        renameSync(
          path.join(output, 'docs.html'),
          path.join(output, 'index.html'),
        );
        copyFileSync(
          'public/yuanheng-logo.png',
          path.join(output, 'yuanheng-logo.png'),
        );
        cpSync('public/docs-images', path.join(output, 'docs-images'), {
          recursive: true,
        });
        writeFileSync(
          path.join(output, 'build-info.json'),
          JSON.stringify(
            {
              project: 'new-api',
              version: readFileSync('../VERSION', 'utf8').trim(),
              revision: execFileSync('git', ['rev-parse', 'HEAD'], {
                encoding: 'utf8',
              }).trim(),
            },
            null,
            2,
          ) + '\n',
        );
        writeFileSync(
          path.join(output, 'robots.txt'),
          'User-agent: *\nAllow: /\nDisallow: /api/\n',
        );
      },
    },
  ],
});
