const remote = '(?:https?|wss?):\\/\\/';
const quote = `["'\\x60]`;

const patterns = {
  html: [new RegExp(`(?:src|href)\\s*=\\s*["']${remote}`, 'i')],
  css: [
    new RegExp(`url\\(\\s*["']?${remote}`, 'i'),
    new RegExp(`@import\\s+(?:url\\()?\\s*["']?${remote}`, 'i'),
  ],
  js: [
    new RegExp(
      `(?:fetch|import|WebSocket|EventSource|Worker|SharedWorker)\\s*\\(\\s*${quote}${remote}`,
      'i',
    ),
    new RegExp(`\\.open\\s*\\([^,]+,\\s*${quote}${remote}`, 'i'),
  ],
};

export function containsRemoteLoad(extension, source) {
  return (patterns[extension] ?? []).some((pattern) => pattern.test(source));
}
