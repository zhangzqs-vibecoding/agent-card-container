import { expect, test } from '@playwright/test';
import { RuntimeFixture } from './support/runtime-fixture';

test.describe('CodeCard local runtime', () => {
  let runtime: RuntimeFixture;

  test.beforeEach(async () => {
    runtime = await RuntimeFixture.start();
  });

  test.afterEach(async () => {
    await runtime?.close();
  });

  test('keeps storage and RPC working while cloud connectivity is offline', async ({ page }) => {
    const externalRequests: string[] = [];
    page.on('request', (request) => {
      if (!new URL(request.url()).hostname.endsWith('.localhost')) externalRequests.push(request.url());
    });

    await page.goto(runtime.url);
    await expect(page.getByTestId('context')).toHaveText('zh-CN|dark|workspace|offline');
	await expect(page.getByTestId('status')).toHaveText('ready');
    await page.getByTestId('note').fill('离线内容');
    await page.getByTestId('save').click();
    await expect(page.getByTestId('status')).toHaveText('saved');
    await page.reload();
    await expect(page.getByTestId('note')).toHaveValue('离线内容');

    await page.getByTestId('clipboard').click();
    await expect(page.getByTestId('status')).toHaveText('clipboard:accepted');
	await runtime.setLightTheme();
	await expect(page.getByTestId('status')).toHaveText('theme:light');
    const blocked = await page.evaluate(async () => {
      try {
        await fetch('https://example.com/forbidden');
        return false;
      } catch {
        return true;
      }
    });
    expect(blocked).toBe(true);
    expect(externalRequests).toEqual([]);
  });

  test('rotates origin and token while restoring host storage after restart', async ({ page, request }) => {
    await page.goto(runtime.url);
	await expect(page.getByTestId('status')).toHaveText('ready');
    await page.getByTestId('note').fill('persisted');
    await page.getByTestId('save').click();
	await expect(page.getByTestId('status')).toHaveText('saved');
	expect(await page.evaluate(() => window.agentCard.invoke('storage.get', { key: 'note' }))).toEqual({ value: 'persisted' });

    const oldUrl = runtime.url;
    const oldOrigin = new URL(oldUrl).origin;
	const bootstrap = await page.evaluate(() => fetch('/runtime/bootstrap.js').then((response) => response.text()));
	const token = bootstrap.match(/const token = "([^"]+)"/)?.[1];
    expect(token).toBeTruthy();

    const next = await runtime.restart();
    expect(new URL(next).origin).not.toBe(oldOrigin);
	const oldTarget = directTarget(oldUrl);
	let oldAuthorityRejected = false;
	try {
	  oldAuthorityRejected = (await request.get(oldTarget.url, { headers: { Host: oldTarget.host } })).status() !== 200;
	} catch {
	  oldAuthorityRejected = true;
	}
	expect(oldAuthorityRejected).toBe(true);

    await page.goto(next);
	expect(await page.evaluate(() => window.agentCard.invoke('storage.get', { key: 'note' }))).toEqual({ value: 'persisted' });
    await expect(page.getByTestId('note')).toHaveValue('persisted');
	const nextTarget = directTarget(next, '/v1/rpc');
	const staleToken = await request.post(nextTarget.url, {
      headers: {
        Authorization: `Bearer ${token}`,
        Origin: new URL(next).origin,
        'X-AgentCard-RPC-Version': '1',
		Host: nextTarget.host,
      },
      data: { jsonrpc: '2.0', id: 1, method: 'runtime.getContext', params: {} },
    });
    expect(staleToken.status()).toBe(401);
  });

  test('rejects missing credentials, foreign authorities, and closed sessions', async ({ request }) => {
    const origin = new URL(runtime.url).origin;
	const target = directTarget(runtime.url, '/v1/rpc');
	const unauthorized = await request.post(target.url, {
	  headers: { Host: target.host, Origin: origin, 'X-AgentCard-RPC-Version': '1' },
      data: { jsonrpc: '2.0', id: 1, method: 'runtime.getContext', params: {} },
    });
    expect(unauthorized.status()).toBe(401);

	const foreignHost = `foreign-${target.host}`;
	expect((await request.get(target.url, { headers: { Host: foreignHost } })).status()).toBe(404);

    await runtime.closeSession();
	expect((await request.get(directTarget(runtime.url).url, { headers: { Host: target.host } })).status()).toBe(404);
  });
});

function directTarget(sessionUrl: string, path = new URL(sessionUrl).pathname): { url: string; host: string } {
	const session = new URL(sessionUrl);
	return {
		url: `http://127.0.0.1:${session.port}${path}`,
		host: session.host,
	};
}
