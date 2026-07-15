import { spawn, type ChildProcessWithoutNullStreams } from 'node:child_process';
import { createInterface, type Interface } from 'node:readline';
import { fileURLToPath } from 'node:url';
import path from 'node:path';

type FixtureEvent = { event: string; url?: string; message?: string };

export class RuntimeFixture {
  private readonly waiters = new Map<string, Array<(event: FixtureEvent) => void>>();
  private stderr = '';

  private constructor(
    private readonly process: ChildProcessWithoutNullStreams,
    private readonly lines: Interface,
    public url: string,
  ) {
    lines.on('line', (line) => {
      let event: FixtureEvent;
      try {
        event = JSON.parse(line) as FixtureEvent;
      } catch {
        this.stderr += `invalid fixture output: ${line}\n`;
        return;
      }
      this.waiters.get(event.event)?.shift()?.(event);
    });
    process.stderr.on('data', (chunk) => {
      this.stderr += chunk.toString();
      if (this.stderr.length > 8_192) this.stderr = this.stderr.slice(-8_192);
    });
  }

  static async start(): Promise<RuntimeFixture> {
    const browserRoot = path.dirname(path.dirname(path.dirname(fileURLToPath(import.meta.url))));
    const repository = path.resolve(browserRoot, '..', '..');
    const desktop = path.join(repository, 'apps', 'desktop');
    const script = path.join(browserRoot, 'fixture_server.dart');
    const child = spawn(
      'dart',
      ['run', `--packages=${path.join(desktop, '.dart_tool', 'package_config.json')}`, script],
      { cwd: desktop, stdio: ['pipe', 'pipe', 'pipe'] },
    );
    const lines = createInterface({ input: child.stdout });
    const fixture = new RuntimeFixture(child, lines, '');
    const ready = await fixture.waitFor('ready');
    if (!ready.url) throw new Error('runtime fixture returned no URL');
    fixture.url = ready.url;
    return fixture;
  }

  async restart(): Promise<string> {
    const ready = this.waitFor('ready');
    this.process.stdin.write('restart\n');
    const event = await ready;
    if (!event.url) throw new Error('runtime fixture returned no URL');
    this.url = event.url;
    return this.url;
  }

  async closeSession(): Promise<void> {
    const closed = this.waitFor('session-closed');
    this.process.stdin.write('close-session\n');
    await closed;
  }

  async setLightTheme(): Promise<void> {
    const updated = this.waitFor('theme-updated');
    this.process.stdin.write('theme-light\n');
    await updated;
  }

  async close(): Promise<void> {
    if (this.process.exitCode !== null) return;
    const exited = new Promise<void>((resolve) => this.process.once('exit', () => resolve()));
    this.process.stdin.write('shutdown\n');
    await Promise.race([
      exited,
      new Promise<never>((_, reject) => setTimeout(() => reject(new Error(`fixture did not exit: ${this.stderr}`)), 5_000)),
    ]).catch((error) => {
      this.process.kill('SIGKILL');
      throw error;
    });
    this.lines.close();
  }

  private waitFor(eventName: string): Promise<FixtureEvent> {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`fixture event ${eventName} timed out: ${this.stderr}`)), 10_000);
      const waiter = (event: FixtureEvent) => {
        clearTimeout(timer);
        resolve(event);
      };
      const queue = this.waiters.get(eventName) ?? [];
      queue.push(waiter);
      this.waiters.set(eventName, queue);
      if (this.process.exitCode !== null) {
        clearTimeout(timer);
        reject(new Error(`fixture exited with ${this.process.exitCode}: ${this.stderr}`));
      }
    });
  }
}
