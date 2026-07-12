import { readFile } from 'node:fs/promises';
import { join } from 'node:path';

const root = process.cwd();
const readJson = async (path) => JSON.parse(await readFile(path, 'utf8'));
const manifest = await readJson(join(root, 'package.json'));
const policy = await readJson(join(root, 'dependency-policy.json'));

if (manifest.packageManager !== policy.packageManager &&
    !manifest.packageManager?.startsWith(`${policy.packageManager}+`)) {
  throw new Error('Package manager does not match dependency policy');
}
if (!Array.isArray(policy.allowedLicenses) || !Array.isArray(policy.productionDependencies)) {
  throw new Error('Dependency policy is invalid');
}

const declared = manifest.dependencies ?? {};
const audited = new Map(policy.productionDependencies.map((entry) => [entry.name, entry]));
if (Object.keys(declared).length !== audited.size) {
  throw new Error('Production dependency set does not match policy');
}
for (const [name, version] of Object.entries(declared)) {
  const entry = audited.get(name);
  if (!entry || entry.version !== version || !/^\d+\.\d+\.\d+$/.test(version)) {
    throw new Error(`Dependency ${name} is not pinned to its audited version`);
  }
  if (!policy.allowedLicenses.includes(entry.license)) {
    throw new Error(`Dependency ${name} has a forbidden policy license`);
  }
  const installed = await readJson(join(root, 'node_modules', name, 'package.json'));
  if (installed.version !== version || installed.license !== entry.license) {
    throw new Error(`Installed dependency ${name} does not match policy`);
  }
}

if (policy.audit?.command !== 'pnpm audit --audit-level high' ||
    policy.audit?.highOrCriticalVulnerabilities !== 0) {
  throw new Error('Dependency audit policy is not fail-closed');
}
