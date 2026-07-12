import { describe, expect, it } from 'vitest';

import { containsRemoteLoad } from './bundle-policy.mjs';

describe('containsRemoteLoad', () => {
  it('allows inert XML namespace identifiers bundled by Preact', () => {
    expect(
      containsRemoteLoad(
        'js',
        'const svgNamespace = "http://www.w3.org/2000/svg";',
      ),
    ).toBe(false);
  });

  it('rejects executable remote loads in HTML, CSS and JavaScript', () => {
    expect(containsRemoteLoad('html', '<script src="https://evil.test/x.js"></script>')).toBe(true);
    expect(containsRemoteLoad('css', 'body{background:url(https://evil.test/a.png)}')).toBe(true);
    expect(containsRemoteLoad('js', 'fetch("https://evil.test/data")')).toBe(true);
    expect(containsRemoteLoad('js', 'new WebSocket("wss://evil.test/socket")')).toBe(true);
  });
});
