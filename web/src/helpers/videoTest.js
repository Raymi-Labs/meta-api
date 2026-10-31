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

export const isWanImageModel = (name = '') => {
  const model = name.trim().toLowerCase().split('/').pop();
  return (
    !model
      .split('-')
      .some((part) =>
        [
          't2v',
          'i2v',
          'r2v',
          'v2v',
          's2v',
          'flf2v',
          'video',
          'videoedit',
        ].includes(part),
      ) &&
    /^wanx?(?:[0-9]+(?:\.[0-9]+)*)?-(?:image(?:-pro)?|t2i|i2i|imageedit)(?:$|-)/.test(
      model,
    )
  );
};

// Conservative UI hints only. The server independently enforces the no-generation test path.
export const isVideoTestModel = (name = '', channelType = 0, endpoint = '') => {
  if (
    endpoint === 'openai-video' ||
    [50, 52, 54, 55].includes(Number(channelType))
  )
    return true;
  const model = name.trim().toLowerCase().split('/').pop();
  if (isWanImageModel(model)) return false;
  return (
    /^(?:grok-imagine-video|sora|veo|wanx?|happyhorse|kling|hailuo|minimax-h3|minimax-hailuo|minimax-video|seedance|doubao-seedance|vidu|cogvideox?|jimeng-video|luma|ray-2|ray-3|runway)(?:$|[-.])/.test(
      model,
    ) || /^(?:wanx?|veo)\d/.test(model)
  );
};

export const isLiveTestSuccess = (result) =>
  Boolean(result?.success && result.testKind !== 'video_preflight');

export const summarizeModelTests = (results) =>
  results.reduce(
    (counts, result) => {
      if (result?.testKind === 'video_preflight') counts.preflight++;
      else if (result?.success) counts.success++;
      else counts.failed++;
      return counts;
    },
    { success: 0, failed: 0, preflight: 0 },
  );
