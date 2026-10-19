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

// Published, signed APK. Update only after the mobile delivery workflow verifies it.
const version = '0.1.0-alpha.1';
const filename = `YuanHeng-${version}-android-arm64.apk`;
const base = `https://download.meta-api.vip/mobile/releases/${version}`;

export const androidDownload = Object.freeze({
  version,
  filename,
  size: 16984529,
  sha256: '499f09f38d01c75fab1cc419d6e8a57e3af8c57ff241dda699cd67fa4b1d977e',
  url: `${base}/${filename}`,
  checksumURL: `${base}/${filename}.sha256`,
  backupURL: `https://github.com/nanashiwang/yuanheng-switch/releases/download/mobile-v${version}/${filename}`,
});
