import assert from 'node:assert/strict';
import test from 'node:test';

import {
  buildBulkPayload,
  evaluateBulkAdvisories,
  requestBulkAdvisories,
} from './audit-dependencies.mjs';

test('buildBulkPayload includes direct and transitive installed versions', () => {
  const payload = buildBulkPayload([
    {
      name: 'root',
      version: '1.0.0',
      dependencies: {
        preact: {
          version: '10.26.4',
          dependencies: {
            nested: {version: '2.0.0'},
          },
        },
      },
      devDependencies: {
        vite: {version: '6.4.3'},
      },
    },
  ]);

  assert.deepEqual(payload, {
    nested: ['2.0.0'],
    preact: ['10.26.4'],
    vite: ['6.4.3'],
  });
});

test('evaluateBulkAdvisories rejects high and critical advisories only', () => {
  assert.deepEqual(
    evaluateBulkAdvisories({
      low: [{id: 1, severity: 'low'}],
      moderate: [{id: 2, severity: 'moderate'}],
    }),
    [],
  );

  assert.deepEqual(
    evaluateBulkAdvisories({
      critical: [{id: 3, severity: 'critical'}],
      high: [{id: 4, severity: 'high'}],
    }),
    [
      {name: 'critical', id: 3, severity: 'critical'},
      {name: 'high', id: 4, severity: 'high'},
    ],
  );
});

test('evaluateBulkAdvisories fails closed on an unknown severity', () => {
  assert.throws(
    () => evaluateBulkAdvisories({preact: [{id: 5, severity: 'unknown'}]}),
    /unknown severity/,
  );
});

test('requestBulkAdvisories fails closed on an unavailable endpoint', async () => {
  await assert.rejects(
    requestBulkAdvisories(
      {preact: ['10.26.4']},
      async () => new Response('retired', {status: 410}),
      'https://registry.example/',
    ),
    /status 410/,
  );
});

test('requestBulkAdvisories posts the bulk payload to the official path', async () => {
  let captured;
  const advisories = await requestBulkAdvisories(
    {preact: ['10.26.4']},
    async (url, options) => {
      captured = {url, options};
      return Response.json({});
    },
    'https://registry.npmjs.org',
  );

  assert.deepEqual(advisories, {});
  assert.equal(
    captured.url,
    'https://registry.npmjs.org/-/npm/v1/security/advisories/bulk',
  );
  assert.equal(captured.options.method, 'POST');
  assert.deepEqual(JSON.parse(captured.options.body), {preact: ['10.26.4']});
});
