import {execFileSync} from 'node:child_process';
import {fileURLToPath} from 'node:url';

const officialRegistry = 'https://registry.npmjs.org';
const dependencyFields = [
  'dependencies',
  'devDependencies',
  'optionalDependencies',
];
const knownSeverities = new Set(['info', 'low', 'moderate', 'high', 'critical']);

export function buildBulkPayload(projects) {
  const versions = new Map();

  function visitCollection(collection) {
    if (!collection || typeof collection !== 'object' || Array.isArray(collection)) {
      return;
    }
    for (const [name, dependency] of Object.entries(collection)) {
      if (!dependency || typeof dependency !== 'object' || Array.isArray(dependency)) {
        throw new Error(`invalid dependency tree entry for ${name}`);
      }
      if (typeof dependency.version !== 'string' || dependency.version.length === 0) {
        throw new Error(`dependency ${name} has no installed version`);
      }
      const installed = versions.get(name) ?? new Set();
      installed.add(dependency.version);
      versions.set(name, installed);
      for (const field of dependencyFields) {
        visitCollection(dependency[field]);
      }
    }
  }

  if (!Array.isArray(projects)) {
    throw new Error('pnpm dependency tree must be an array');
  }
  for (const project of projects) {
    if (!project || typeof project !== 'object' || Array.isArray(project)) {
      throw new Error('pnpm dependency tree contains an invalid project');
    }
    for (const field of dependencyFields) {
      visitCollection(project[field]);
    }
  }

  return Object.fromEntries(
    [...versions.entries()]
      .sort(([left], [right]) => left.localeCompare(right))
      .map(([name, installed]) => [name, [...installed].sort()]),
  );
}

export function evaluateBulkAdvisories(advisories) {
  if (!advisories || typeof advisories !== 'object' || Array.isArray(advisories)) {
    throw new Error('bulk advisory response must be an object');
  }
  const findings = [];
  for (const [name, entries] of Object.entries(advisories).sort(([left], [right]) =>
    left.localeCompare(right),
  )) {
    if (!Array.isArray(entries)) {
      throw new Error(`bulk advisory response for ${name} must be an array`);
    }
    for (const advisory of entries) {
      if (!advisory || typeof advisory !== 'object' || Array.isArray(advisory)) {
        throw new Error(`bulk advisory response for ${name} is invalid`);
      }
      const {id, severity} = advisory;
      if ((typeof id !== 'string' && typeof id !== 'number') || typeof severity !== 'string') {
        throw new Error(`bulk advisory response for ${name} is incomplete`);
      }
      if (!knownSeverities.has(severity)) {
        throw new Error(`bulk advisory response for ${name} has unknown severity`);
      }
      if (severity === 'high' || severity === 'critical') {
        findings.push({name, id, severity});
      }
    }
  }
  return findings;
}

export async function requestBulkAdvisories(
  payload,
  fetchImplementation = fetch,
  registry = officialRegistry,
) {
  if (!payload || typeof payload !== 'object' || Object.keys(payload).length === 0) {
    throw new Error('dependency audit payload is empty');
  }
  const endpoint = `${registry.replace(/\/+$/, '')}/-/npm/v1/security/advisories/bulk`;
  const response = await fetchImplementation(endpoint, {
    method: 'POST',
    headers: {'content-type': 'application/json'},
    body: JSON.stringify(payload),
    signal: AbortSignal.timeout(30_000),
  });
  if (!response.ok) {
    throw new Error(`bulk advisory endpoint returned status ${response.status}`);
  }
  return response.json();
}

async function main() {
  const tree = JSON.parse(
    execFileSync('pnpm', ['list', '--json', '--depth', 'Infinity'], {
      encoding: 'utf8',
      stdio: ['ignore', 'pipe', 'inherit'],
    }),
  );
  const payload = buildBulkPayload(tree);
  const advisories = await requestBulkAdvisories(payload);
  const findings = evaluateBulkAdvisories(advisories);
  if (findings.length > 0) {
    for (const finding of findings) {
      console.error(
        `dependency audit rejected ${finding.name}: advisory=${finding.id} severity=${finding.severity}`,
      );
    }
    process.exitCode = 1;
    return;
  }
  console.log(
    `dependency bulk audit passed: packages=${Object.keys(payload).length} highOrCritical=0`,
  );
}

if (process.argv[1] && fileURLToPath(import.meta.url) === process.argv[1]) {
  main().catch((error) => {
    console.error(`dependency bulk audit failed: ${error.message}`);
    process.exitCode = 1;
  });
}
