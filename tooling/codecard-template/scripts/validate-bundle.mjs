import { readdir, readFile, stat } from 'node:fs/promises';
import { join, relative, sep } from 'node:path';

const root = join(process.cwd(), 'dist');
const files = [];

async function walk(directory) {
  for (const name of await readdir(directory)) {
    const absolute = join(directory, name);
    const metadata = await stat(absolute);
    if (metadata.isDirectory()) {
      await walk(absolute);
    } else if (metadata.isFile()) {
      files.push(absolute);
    } else {
      throw new Error('Bundle contains a non-regular file');
    }
  }
}

await walk(root);

if (files.length > 512) {
  throw new Error('Bundle contains too many files');
}

let total = 0;
for (const absolute of files) {
  const metadata = await stat(absolute);
  total += metadata.size;
  const path = relative(root, absolute).split(sep).join('/');
  if (metadata.size > 8 * 1024 * 1024 || path.split('/').length > 8) {
    throw new Error('Bundle file exceeds policy');
  }
  if (path.endsWith('.wasm')) {
    throw new Error('WebAssembly is not supported');
  }
  if (/\.(html|js|css)$/.test(path)) {
    const source = await readFile(absolute, 'utf8');
    if (/https?:\/\//i.test(source)) {
      throw new Error('Remote resources are forbidden');
    }
    if (/serviceWorker|navigator\.serviceWorker/.test(source)) {
      throw new Error('Service Worker is forbidden');
    }
  }
}

if (total > 32 * 1024 * 1024) {
  throw new Error('Bundle exceeds expanded size policy');
}

const index = await readFile(join(root, 'index.html'), 'utf8');
const bootstrap = index.indexOf('/runtime/bootstrap.js');
const moduleScript = index.indexOf('type="module"');
if (bootstrap < 0 || moduleScript < 0 || bootstrap > moduleScript) {
  throw new Error('Runtime bootstrap must load before card scripts');
}
