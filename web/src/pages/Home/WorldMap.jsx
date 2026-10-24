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

import React, { useEffect, useRef } from 'react';
import { attachWorldMapMotion } from './worldMapMotion';

export default function WorldMap() {
  const rootRef = useRef(null);
  useEffect(() => {
    const root = rootRef.current;
    return attachWorldMapMotion({
      root,
      svg: root.querySelector('svg'),
      canvas: root.querySelector('canvas'),
      hero: root.closest('.yh-hero'),
    });
  }, []);

  return (
    <div className='yh-world-map' ref={rootRef} aria-hidden='true'>
      <svg
        className='yh-world-map-svg'
        viewBox='0 0 1440 520'
        fill='none'
        aria-hidden='true'
      >
        <defs>
          <pattern
            id='yh-map-dots'
            width='7'
            height='7'
            patternUnits='userSpaceOnUse'
          >
            <circle cx='2' cy='2' r='1.25' fill='currentColor' />
          </pattern>
          <radialGradient id='yh-map-fade'>
            <stop offset='0' stopColor='white' stopOpacity='.12' />
            <stop offset='.65' stopColor='white' stopOpacity='.72' />
            <stop offset='1' stopColor='white' stopOpacity='.15' />
          </radialGradient>
          <mask id='yh-map-mask'>
            <rect width='1440' height='520' fill='url(#yh-map-fade)' />
          </mask>
        </defs>
        <g
          className='yh-map-land'
          fill='url(#yh-map-dots)'
          mask='url(#yh-map-mask)'
        >
          <path d='M83 115 130 80 210 56 276 72 300 60 354 87 380 115 347 140 330 183 298 199 276 232 246 218 216 185 184 187 157 157 123 155 96 137ZM221 211 252 227 281 262 312 277 298 290 265 276 243 248ZM325 280 359 271 404 291 443 331 424 364 406 384 393 435 363 468 347 425 328 388 321 348 305 311Z' />
          <path d='M394 42 430 28 495 41 480 77 447 111 412 99 402 70ZM661 108 696 87 730 87 739 118 771 137 755 157 714 151 687 174 653 160 635 136ZM651 184 693 165 746 181 788 216 790 259 764 305 738 356 701 359 686 314 657 285 642 243 622 216Z' />
          <path d='M748 92 811 67 878 78 917 64 1006 80 1054 63 1127 92 1182 92 1256 125 1219 153 1180 150 1143 182 1105 183 1091 213 1050 231 1028 262 993 258 966 220 938 204 925 247 898 266 874 233 858 204 817 199 787 169 761 150ZM1013 274 1050 287 1082 322 1133 330 1164 354 1123 366 1083 349 1048 335ZM1208 197 1221 214 1210 242 1194 236ZM1174 378 1235 348 1271 361 1302 403 1283 438 1229 443 1188 428 1155 405ZM1321 449 1344 429 1348 456 1325 483Z' />
        </g>
        <g
          className='yh-map-routes'
          stroke='currentColor'
          strokeWidth='1'
          opacity='.16'
        >
          <path d='M278 168 Q690 -30 1080 190' strokeDasharray='4 7' />
          <path d='M278 168 Q700 395 1080 190' strokeDasharray='4 7' />
        </g>
        {[
          [278, 168],
          [704, 138],
          [1080, 190],
        ].map(([cx, cy]) => (
          <g key={cx}>
            <circle cx={cx} cy={cy} r='12' fill='currentColor' opacity='.08' />
            <circle cx={cx} cy={cy} r='3' fill='currentColor' opacity='.5' />
          </g>
        ))}
      </svg>
      <canvas className='yh-world-map-canvas' />
    </div>
  );
}
