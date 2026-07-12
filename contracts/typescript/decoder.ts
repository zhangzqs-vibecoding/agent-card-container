export type CardRuntime = 'native' | 'web';

export interface CardFile {
  path: string;
  sha256: string;
  size: number;
}

export interface CardDefinition {
  formatVersion: 1;
  cardId: string;
  versionId: string;
  runtime: CardRuntime;
  files: CardFile[];
}

export interface NativeNode {
  id: string;
  type: string;
  children: NativeNode[];
}

export interface NativeCard {
  schemaVersion: 1;
  root: NativeNode;
}

export interface LocalRpcContract {
  contractVersion: number;
  methods: string[];
  events: string[];
  stableErrors: string[];
}

const componentTypes = new Set([
  'Container', 'Row', 'Column', 'Stack', 'Grid', 'Scroll', 'Divider',
  'Text', 'Icon', 'Image', 'Badge', 'Progress', 'Chart', 'Button',
  'TextInput', 'Checkbox', 'Select', 'Slider', 'List', 'KeyValue',
  'EmptyState', 'ErrorState',
]);

export function decodeCardDefinition(value: unknown): CardDefinition {
  const input = object(value, 'CardDefinition');
  const formatVersion = integer(input.formatVersion, 'formatVersion');
  if (formatVersion !== 1) throw new Error('formatVersion must be 1');
  const runtime = text(input.runtime, 'runtime');
  if (runtime !== 'native' && runtime !== 'web') throw new Error('runtime is invalid');
  const files = array(input.files, 'files').map((value, index) => {
    const file = object(value, `files[${index}]`);
    const sha256 = text(file.sha256, `files[${index}].sha256`);
    if (!/^[a-f0-9]{64}$/.test(sha256)) throw new Error('file sha256 is invalid');
    const size = integer(file.size, `files[${index}].size`);
    if (size < 0 || size > 8 * 1024 * 1024) throw new Error('file size is invalid');
    return { path: text(file.path, `files[${index}].path`), sha256, size };
  });
  return {
    formatVersion: 1,
    cardId: text(input.cardId, 'cardId'),
    versionId: text(input.versionId, 'versionId'),
    runtime,
    files,
  };
}

export function decodeNativeCard(value: unknown): NativeCard {
  const input = object(value, 'NativeCard');
  if (integer(input.schemaVersion, 'schemaVersion') !== 1) {
    throw new Error('schemaVersion must be 1');
  }
  const counter = { value: 0 };
  return { schemaVersion: 1, root: decodeNode(input.root, 1, counter) };
}

export function decodeLocalRpcContract(value: unknown): LocalRpcContract {
  const input = object(value, 'LocalRpcContract');
  return {
    contractVersion: integer(input.contractVersion, 'contractVersion'),
    methods: stringArray(input.methods, 'methods'),
    events: stringArray(input.events, 'events'),
    stableErrors: stringArray(input.stableErrors, 'stableErrors'),
  };
}

function decodeNode(value: unknown, depth: number, counter: { value: number }): NativeNode {
  if (depth > 32) throw new Error('NativeCard exceeds depth 32');
  counter.value++;
  if (counter.value > 500) throw new Error('NativeCard exceeds 500 nodes');
  const input = object(value, 'node');
  const type = text(input.type, 'node.type');
  if (!componentTypes.has(type)) throw new Error('NativeCard component is invalid');
  const children = input.children === undefined ? [] : array(input.children, 'node.children');
  if (children.length > 200) throw new Error('node exceeds 200 children');
  return {
    id: text(input.id, 'node.id'),
    type,
    children: children.map((child) => decodeNode(child, depth + 1, counter)),
  };
}

function object(value: unknown, label: string): Record<string, unknown> {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
    throw new Error(`${label} must be an object`);
  }
  return value as Record<string, unknown>;
}

function array(value: unknown, label: string): unknown[] {
  if (!Array.isArray(value)) throw new Error(`${label} must be an array`);
  return value;
}

function text(value: unknown, label: string): string {
  if (typeof value !== 'string' || value.length === 0) {
    throw new Error(`${label} must be a non-empty string`);
  }
  return value;
}

function integer(value: unknown, label: string): number {
  if (typeof value !== 'number' || !Number.isInteger(value)) {
    throw new Error(`${label} must be an integer`);
  }
  return value;
}

function stringArray(value: unknown, label: string): string[] {
  return array(value, label).map((entry, index) => text(entry, `${label}[${index}]`));
}
