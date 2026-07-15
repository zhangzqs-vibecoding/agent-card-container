import 'dart:convert';
import 'dart:io';
import 'dart:typed_data';

import '../../apps/desktop/lib/src/runtime/local_runtime_server.dart';
import '../../apps/desktop/lib/src/storage/database_runtime_storage.dart';
import '../../apps/desktop/lib/src/storage/local_database.dart';

const _index = '''<!doctype html>
<html lang="zh-CN">
  <head><meta charset="UTF-8"><title>CodeCard Runtime Gate</title></head>
  <body>
    <output data-testid="context">loading</output>
    <input data-testid="note" aria-label="note">
    <button data-testid="save">save</button>
    <button data-testid="clipboard">clipboard</button>
    <output data-testid="status">idle</output>
    <script src="/runtime/bootstrap.js"></script>
    <script type="module" src="/app.js"></script>
  </body>
</html>''';

const _application = r'''
const context = await window.agentCard.getContext();
document.querySelector('[data-testid="context"]').textContent =
  `${context.locale}|${context.theme}|${context.surface}|${context.online ? 'online' : 'offline'}`;

const note = document.querySelector('[data-testid="note"]');
const status = document.querySelector('[data-testid="status"]');
const stored = await window.agentCard.invoke('storage.get', {key: 'note'});
note.value = typeof stored.value === 'string' ? stored.value : '';

document.querySelector('[data-testid="save"]').addEventListener('click', async () => {
  await window.agentCard.invoke('storage.set', {key: 'note', value: note.value});
  status.textContent = 'saved';
});
document.querySelector('[data-testid="clipboard"]').addEventListener('click', async () => {
  const result = await window.agentCard.invoke('clipboard.write', {text: note.value});
  status.textContent = `clipboard:${result.status}`;
});
window.agentCard.subscribe('theme.changed', (event) => {
  status.textContent = `theme:${event.theme}`;
});
status.textContent = 'ready';
''';

Future<void> main() async {
  final directory = await Directory.systemTemp.createTemp(
    'agent-card-browser-',
  );
  final database = LocalDatabase.open('${directory.path}/runtime.sqlite3');
  var server = await LocalRuntimeServer.start(
    initialLocale: 'zh-CN',
    initialTheme: 'dark',
    initialOnline: false,
  );
  RuntimeSession? session;

  RuntimeSession createSession() {
    final created = server.createSession(
      instanceId: 'browser-instance',
      cardId: 'browser-card',
      versionId: 'browser-version',
      resources: {
        '/index.html': RuntimeResource.html(_index),
        '/app.js': RuntimeResource(
          bytes: Uint8List.fromList(utf8.encode(_application)),
          contentType: 'application/javascript; charset=utf-8',
        ),
      },
      declaredCapabilities: const {'storage', 'clipboard.write'},
      locale: 'zh-CN',
      theme: 'dark',
      surface: 'workspace',
      online: false,
      storage: DatabaseRuntimeStorage(database, 'browser-state'),
      rpcHandler: (_, method, params) {
        if (method != 'clipboard.write' || params['text'] is! String) {
          throw const RuntimeRpcException(
            'INVALID_PARAMS',
            'Unsupported fixture invocation',
          );
        }
        return {'status': 'accepted'};
      },
    );
    session = created;
    stdout.writeln(
      jsonEncode({'event': 'ready', 'url': '${created.origin}/index.html'}),
    );
    return created;
  }

  createSession();
  await for (final line
      in stdin.transform(utf8.decoder).transform(const LineSplitter())) {
    switch (line) {
      case 'restart':
        await server.close();
        server = await LocalRuntimeServer.start(
          initialLocale: 'zh-CN',
          initialTheme: 'dark',
          initialOnline: false,
        );
        createSession();
      case 'theme-light':
        server.updateEnvironment(theme: 'light');
        stdout.writeln(jsonEncode({'event': 'theme-updated'}));
      case 'close-session':
        final current = session;
        if (current != null) server.closeSession(current.id);
        session = null;
        stdout.writeln(jsonEncode({'event': 'session-closed'}));
      case 'shutdown':
        await server.close();
        database.close();
        await directory.delete(recursive: true);
        stdout.writeln(jsonEncode({'event': 'shutdown'}));
        return;
      default:
        stderr.writeln('unsupported fixture command');
    }
  }
  await server.close();
}
