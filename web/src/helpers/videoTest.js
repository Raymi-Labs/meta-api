// Conservative UI hints only. The server independently enforces the no-generation test path.
export const isVideoTestModel = (name = '', channelType = 0, endpoint = '') => {
  if (
    endpoint === 'openai-video' ||
    [50, 52, 54, 55].includes(Number(channelType))
  )
    return true;
  const model = name.trim().toLowerCase().split('/').pop();
  return (
    /^(?:grok-imagine-video|sora|veo|wanx?|happyhorse|kling|hailuo|minimax-hailuo|minimax-video|seedance|doubao-seedance|vidu|cogvideox?|jimeng-video|luma|ray-2|ray-3|runway)(?:$|[-.])/.test(
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
