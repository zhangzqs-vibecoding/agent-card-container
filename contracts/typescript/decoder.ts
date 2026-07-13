import { nativeCardCatalog } from './native-card-catalog.generated.js';

export type CardRuntime = 'native' | 'web';

export const nativeCatalogSemantics = nativeCardCatalog;

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
  props: Record<string, unknown>;
  events: Record<string, Record<string, unknown>[]>;
  children: NativeNode[];
}

export interface NativeCard {
  schemaVersion: 1;
  initialState: Record<string, unknown>;
  root: NativeNode;
}

export interface LocalRpcContract {
  contractVersion: number;
  methods: string[];
  events: string[];
  stableErrors: string[];
}

type ValueRule = {
  type: string;
  binding?: boolean;
  enum?: readonly string[];
  minimum?: number;
  maximum?: number;
  minLength?: number;
  pattern?: string;
  uniqueItems?: boolean;
};

type ComponentRule = {
  allowedProps: Record<string, ValueRule>;
  requiredProps: readonly string[];
  allowedEvents: readonly string[];
};

type ActionRule = {
  requiredFields: readonly string[];
  optionalFields: readonly string[];
  valueRule?: ValueRule;
};

type ExpressionRule = {
  minArgs: number;
  maxArgs?: number;
  argumentType: string;
  resultType: string;
  constraint?: string;
};

const componentRules = nativeCardCatalog.components as unknown as Record<string, ComponentRule>;
const actionRules = nativeCardCatalog.actions as unknown as Record<string, ActionRule>;
const expressionRules = nativeCardCatalog.expressions as unknown as Record<string, ExpressionRule>;
const capabilityMethods = nativeCardCatalog.capabilityMethods as unknown as Record<string, {
  manifestCapability: string | null;
  nativeSupported: boolean;
}>;

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
	rejectUnknown(input, ['schemaVersion', 'initialState', 'root'], 'NativeCard');
  if (integer(input.schemaVersion, 'schemaVersion') !== 1) {
	throw new Error('schemaVersion must be 1');
  }
	const initialState = object(input.initialState, 'initialState');
  const counter = { value: 0 };
  return { schemaVersion: 1, initialState, root: decodeNode(input.root, 1, counter) };
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
	rejectUnknown(input, ['id', 'type', 'props', 'events', 'children'], 'node');
  const type = text(input.type, 'node.type');
	const rule = componentRules[type];
  if (!rule) throw new Error('NativeCard component is invalid');
	const props = input.props === undefined ? {} : object(input.props, 'node.props');
	rejectUnknown(props, Object.keys(rule.allowedProps), `${type} props`);
	for (const required of rule.requiredProps) {
	  if (!hasOwn(props, required)) throw new Error(`${type} requires prop ${required}`);
	}
	for (const [name, propValue] of Object.entries(props)) {
	  validateValue(propValue, rule.allowedProps[name], `${type}.${name}`);
	}
	const rawEvents = input.events === undefined ? {} : object(input.events, 'node.events');
	rejectUnknown(rawEvents, rule.allowedEvents, `${type} events`);
	const events: Record<string, Record<string, unknown>[]> = {};
	for (const [name, rawActions] of Object.entries(rawEvents)) {
	  const actions = array(rawActions, `${type}.${name}`);
	  if (actions.length > nativeCardCatalog.limits.maxActionsPerEvent) {
		throw new Error(`${type}.${name} exceeds ${nativeCardCatalog.limits.maxActionsPerEvent} actions`);
	  }
	  events[name] = actions.map((action, index) =>
	    decodeAction(action, `${type}.${name}[${index}]`));
	}
  const children = input.children === undefined ? [] : array(input.children, 'node.children');
  if (children.length > 200) throw new Error('node exceeds 200 children');
  return {
	 id: text(input.id, 'node.id'),
	 type,
	 props,
	 events,
	 children: children.map((child) => decodeNode(child, depth + 1, counter)),
  };
}

function decodeAction(value: unknown, label: string): Record<string, unknown> {
  const input = object(value, label);
  const type = text(input.type, `${label}.type`);
  const rule = actionRules[type];
  if (!rule) throw new Error(`${label} action type is invalid`);
  const allowed = ['type', ...rule.requiredFields, ...rule.optionalFields];
  rejectUnknown(input, allowed, label);
  for (const required of rule.requiredFields) {
	if (!hasOwn(input, required)) throw new Error(`${label} requires ${required}`);
  }
  if (hasOwn(input, 'path')) statePath(input.path, `${label}.path`);
  if (hasOwn(input, 'params')) object(input.params, `${label}.params`);
  if (hasOwn(input, 'value') && rule.valueRule) {
	validateValue(input.value, rule.valueRule, `${label}.value`);
  }
  if (type === 'capability.invoke') {
	const method = text(input.method, `${label}.method`);
	if (!capabilityMethods[method]?.nativeSupported) {
	  throw new Error(`${label}.method is unsupported`);
	}
  }
  return input;
}

function validateValue(value: unknown, rule: ValueRule, label: string): void {
  if (rule.binding) {
	const evaluated = validateEvaluated(value, label);
	if (evaluated.expression) {
	  if (evaluated.type !== 'unknown' && rule.type !== 'any' && evaluated.type !== rule.type) {
		throw new Error(`${label} expression result type is invalid`);
	  }
	  return;
	}
  }
  validateLiteral(value, rule, label);
}

function validateEvaluated(value: unknown, label: string): { type: string; expression: boolean } {
  if (typeof value !== 'object' || value === null || Array.isArray(value)) {
	return { type: literalType(value), expression: false };
  }
  const input = value as Record<string, unknown>;
  const keys = Object.keys(input);
  if (keys.length === 1 && keys[0] === 'path' && typeof input.path === 'string') {
	if (!/^state\.[^.]+(?:\.[^.]+)*$/.test(input.path)) throw new Error(`${label} path is invalid`);
	return { type: 'unknown', expression: true };
  }
  if (keys.length === 1 && keys[0] === 'expr' && isObject(input.expr)) {
	return { type: validateExpression(input.expr, `${label}.expr`), expression: true };
  }
  if (keys.length === 2 && keys.includes('op') && keys.includes('args')) {
	return { type: validateExpression(input, label), expression: true };
  }
  return { type: 'object', expression: false };
}

function validateExpression(input: Record<string, unknown>, label: string): string {
  rejectUnknown(input, ['op', 'args'], label);
  const operation = text(input.op, `${label}.op`);
  const rule = expressionRules[operation];
  if (!rule) throw new Error(`${label}.op is invalid`);
  const args = array(input.args, `${label}.args`);
  if (args.length < rule.minArgs || (rule.maxArgs !== undefined && args.length > rule.maxArgs)) {
	throw new Error(`${label}.args arity is invalid`);
  }
  for (const [index, argument] of args.entries()) {
	const result = validateEvaluated(argument, `${label}.args[${index}]`);
	if (rule.argumentType !== 'any' && result.type !== 'unknown' && result.type !== rule.argumentType) {
	  throw new Error(`${label}.args type is invalid`);
	}
  }
  if (rule.constraint === 'nonNegative' && typeof args[0] === 'number' && args[0] < 0) {
	throw new Error(`${label}.args must be non-negative`);
  }
  if (rule.constraint === 'nonZeroDivisor' && typeof args[1] === 'number' && args[1] === 0) {
	throw new Error(`${label}.args divisor must be non-zero`);
  }
  return rule.resultType;
}

function validateLiteral(value: unknown, rule: ValueRule, label: string): void {
  let valid = false;
  switch (rule.type) {
	case 'any': valid = true; break;
	case 'string': valid = typeof value === 'string'; break;
	case 'number': valid = typeof value === 'number' && Number.isFinite(value); break;
	case 'integer': valid = typeof value === 'number' && Number.isInteger(value); break;
	case 'boolean': valid = typeof value === 'boolean'; break;
	case 'stringList': valid = Array.isArray(value) && value.every((item) => typeof item === 'string'); break;
	case 'numberList': valid = Array.isArray(value) && value.every((item) => typeof item === 'number' && Number.isFinite(item)); break;
	case 'object': valid = isObject(value); break;
	case 'timerConfiguration': {
	  const timer = object(value, label);
	  rejectUnknown(timer, ['intervalMs', 'delta', 'stopAt'], label);
	  valid = Number.isInteger(timer.intervalMs) && (timer.intervalMs as number) > 0 &&
		  typeof timer.delta === 'number' &&
		  (!hasOwn(timer, 'stopAt') || typeof timer.stopAt === 'number');
	  break;
	}
  }
  if (!valid) throw new Error(`${label} must be ${rule.type}`);
  if (rule.enum && !rule.enum.includes(value as string)) throw new Error(`${label} is outside enum`);
  if (typeof value === 'number' && rule.minimum !== undefined && value < rule.minimum) throw new Error(`${label} is below minimum`);
  if (typeof value === 'number' && rule.maximum !== undefined && value > rule.maximum) throw new Error(`${label} is above maximum`);
  if (typeof value === 'string' && rule.minLength !== undefined && value.length < rule.minLength) throw new Error(`${label} is too short`);
  if (typeof value === 'string' && rule.pattern !== undefined && !new RegExp(rule.pattern).test(value)) throw new Error(`${label} has invalid format`);
  if (Array.isArray(value) && rule.uniqueItems && new Set(value).size !== value.length) throw new Error(`${label} must contain unique items`);
}

function literalType(value: unknown): string {
  if (typeof value === 'number') return 'number';
  if (typeof value === 'string') return 'string';
  if (typeof value === 'boolean') return 'boolean';
  if (Array.isArray(value)) return 'array';
  if (isObject(value)) return 'object';
  return 'unknown';
}

function statePath(value: unknown, label: string): string {
  const path = text(value, label);
  if (!/^[^.]+(?:\.[^.]+)*$/.test(path)) throw new Error(`${label} is invalid`);
  return path;
}

function rejectUnknown(input: Record<string, unknown>, allowed: readonly string[], label: string): void {
  const allowedSet = new Set(allowed);
  const unknown = Object.keys(input).filter((key) => !allowedSet.has(key));
  if (unknown.length > 0) throw new Error(`${label} has unknown fields: ${unknown.join(', ')}`);
}

function hasOwn(input: Record<string, unknown>, key: string): boolean {
  return Object.prototype.hasOwnProperty.call(input, key);
}

function isObject(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
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
