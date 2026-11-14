// Advisory only: never transmit a password to estimate its strength.
export function passwordFeedback(password = '', confirmation, maxLength = 20) {
  const length = Array.from(password).length;
  const validLength = length >= 8 && (maxLength == null || length <= maxLength);
  const classes = [/\p{Ll}/u, /\p{Lu}/u, /\p{N}/u, /[^\p{L}\p{N}]/u].filter(
    (rule) => rule.test(password),
  ).length;
  const repetitive = new Set(Array.from(password)).size <= 3;
  const strength =
    !validLength || repetitive || length < 10
      ? 0
      : length >= 12 && classes >= 3
        ? 2
        : 1;
  return {
    length,
    validLength,
    strength,
    matches: confirmation ? password === confirmation : null,
  };
}
