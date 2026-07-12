import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import { pathToFileURL } from 'node:url';
import { join } from 'node:path';

const [decoderPath, root] = process.argv.slice(2);
if (!decoderPath || !root) throw new Error('decoder path and repository root are required');
const decoder = await import(pathToFileURL(decoderPath));
const readJson = async (...parts) =>
  JSON.parse(await readFile(join(root, ...parts), 'utf8'));

const nativeDefinition = decoder.decodeCardDefinition(
  await readJson('contracts', 'card', 'fixtures', 'native-card.json'),
);
const webDefinition = decoder.decodeCardDefinition(
  await readJson('contracts', 'card', 'fixtures', 'web-card.json'),
);
assert.equal(nativeDefinition.runtime, 'native');
assert.equal(webDefinition.runtime, 'web');
assert.equal(nativeDefinition.formatVersion, 1);
assert.ok(webDefinition.files.every((file) => file.path && file.sha256.length === 64));

const nativeCard = decoder.decodeNativeCard(
  await readJson('contracts', 'card', 'fixtures', 'pomodoro-native.json'),
);
assert.equal(nativeCard.schemaVersion, 1);
assert.equal(nativeCard.root.type, 'Container');

const rpc = decoder.decodeLocalRpcContract(
  await readJson('contracts', 'local-rpc', 'contract.json'),
);
assert.equal(rpc.contractVersion, 1);
assert.ok(rpc.methods.includes('runtime.getContext'));
assert.ok(rpc.events.includes('runtime.suspend'));
assert.ok(rpc.stableErrors.includes('PERMISSION_DENIED'));

assert.throws(() => decoder.decodeCardDefinition({
  ...nativeDefinition,
  runtime: 'script',
}), /runtime/);
assert.throws(() => decoder.decodeNativeCard({
  schemaVersion: 1,
  root: { id: 'root', type: 'WebView' },
}), /component/);
assert.throws(() => decoder.decodeLocalRpcContract({
  contractVersion: 1,
  methods: [42],
  events: [],
  stableErrors: [],
}), /methods/);

console.log('TypeScript shared-contract fixtures passed');
