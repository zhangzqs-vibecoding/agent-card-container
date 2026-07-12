import 'dart:convert';

import '../capabilities/capability.dart';
import '../cards/card_instance.dart';
import '../contracts/card_definition.dart';
import '../surfaces/surface.dart';
import 'sqlite_connection.dart';

class LocalDatabase {
  LocalDatabase._(this._connection) {
    _migrate();
  }

  factory LocalDatabase.inMemory() {
    return LocalDatabase._(SqliteConnection.openInMemory());
  }

  factory LocalDatabase.open(String path) {
    return LocalDatabase._(SqliteConnection.open(path));
  }

  final SqliteConnection _connection;

  int get schemaVersion {
    final rows = _connection.query('PRAGMA user_version');
    return rows.single['user_version']! as int;
  }

  void upsertInstallation(CardInstallation installation) {
    _connection.execute(
      '''
      INSERT INTO card_installations (
        version_id, card_id, content_hash, runtime, installed_at, verified
      ) VALUES (?, ?, ?, ?, ?, ?)
      ON CONFLICT(version_id) DO UPDATE SET
        card_id = excluded.card_id,
        content_hash = excluded.content_hash,
        runtime = excluded.runtime,
        installed_at = excluded.installed_at,
        verified = excluded.verified
      ''',
      [
        installation.versionId,
        installation.cardId,
        installation.contentHash,
        installation.runtime.name,
        installation.installedAt.toUtc().toIso8601String(),
        installation.verified,
      ],
    );
  }

  void registerInstallation(StoredInstallation record) {
    final installation = record.installation;
    _connection.execute(
      '''
      INSERT INTO card_installations (
        version_id, card_id, content_hash, runtime, installed_at, verified,
        definition_json, key_id
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(version_id) DO UPDATE SET
        card_id = excluded.card_id,
        content_hash = excluded.content_hash,
        runtime = excluded.runtime,
        installed_at = excluded.installed_at,
        verified = excluded.verified,
        definition_json = excluded.definition_json,
        key_id = excluded.key_id
      ''',
      [
        installation.versionId,
        installation.cardId,
        installation.contentHash,
        installation.runtime.name,
        installation.installedAt.toUtc().toIso8601String(),
        installation.verified,
        jsonEncode(record.definition.toJson()),
        record.keyId,
      ],
    );
  }

  void registerInstalledInstance({
    required StoredInstallation installation,
    required CardSurface surface,
    required CardInstance instance,
    Iterable<PermissionGrant> grants = const [],
  }) {
    _connection.transaction(() {
      registerInstallation(installation);
      upsertSurface(surface);
      upsertInstance(instance);
      for (final grant in grants) {
        upsertGrant(grant);
      }
    });
  }

  StoredInstallation? installation(String versionId) {
    final rows = _connection.query(
      '''
      SELECT *
      FROM card_installations
      WHERE version_id = ? AND definition_json IS NOT NULL
      ''',
      [versionId],
    );
    if (rows.isEmpty) {
      return null;
    }
    return _storedInstallationFromRow(rows.single);
  }

  List<StoredInstallation> listInstallations() {
    return _connection
        .query('''
          SELECT *
          FROM card_installations
          WHERE definition_json IS NOT NULL
          ORDER BY installed_at, version_id
          ''')
        .map(_storedInstallationFromRow)
        .toList(growable: false);
  }

  void upsertSurface(CardSurface surface) {
    final bounds = surface.bounds;
    _connection.execute(
      '''
      INSERT INTO surfaces (
        surface_id, type, monitor_id, x, y, width, height, always_on_top,
        last_focused_at
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(surface_id) DO UPDATE SET
        type = excluded.type,
        monitor_id = excluded.monitor_id,
        x = excluded.x,
        y = excluded.y,
        width = excluded.width,
        height = excluded.height,
        always_on_top = excluded.always_on_top,
        last_focused_at = excluded.last_focused_at
      ''',
      [
        surface.id,
        surface.type.name,
        surface.monitorId,
        bounds?.x,
        bounds?.y,
        bounds?.width,
        bounds?.height,
        surface.alwaysOnTop,
        surface.lastFocusedAt?.toUtc().toIso8601String(),
      ],
    );
  }

  List<CardSurface> listSurfaces() {
    return _connection
        .query('SELECT * FROM surfaces ORDER BY surface_id')
        .map((row) {
          final x = row['x'] as num?;
          final y = row['y'] as num?;
          final width = row['width'] as num?;
          final height = row['height'] as num?;
          return CardSurface(
            id: row['surface_id']! as String,
            type: SurfaceType.values.byName(row['type']! as String),
            monitorId: row['monitor_id'] as String?,
            bounds: x == null || y == null || width == null || height == null
                ? null
                : CardPlacement(
                    x: x.toDouble(),
                    y: y.toDouble(),
                    width: width.toDouble(),
                    height: height.toDouble(),
                  ),
            alwaysOnTop: (row['always_on_top']! as int) != 0,
            lastFocusedAt: row['last_focused_at'] == null
                ? null
                : DateTime.parse(row['last_focused_at']! as String).toUtc(),
          );
        })
        .toList(growable: false);
  }

  void updateSurfaceWindowState(
    String surfaceId, {
    CardPlacement? bounds,
    DateTime? focusedAt,
    String? monitorId,
  }) {
    _connection.execute(
      '''
      UPDATE surfaces SET
        x = COALESCE(?, x),
        y = COALESCE(?, y),
        width = COALESCE(?, width),
        height = COALESCE(?, height),
        last_focused_at = COALESCE(?, last_focused_at),
        monitor_id = COALESCE(?, monitor_id)
      WHERE surface_id = ?
      ''',
      [
        bounds?.x,
        bounds?.y,
        bounds?.width,
        bounds?.height,
        focusedAt?.toUtc().toIso8601String(),
        monitorId,
        surfaceId,
      ],
    );
  }

  void upsertInstance(CardInstance instance) {
    _connection.execute(
      '''
      INSERT INTO card_instances (
        instance_id, card_id, version_id, surface_id,
        x, y, width, height, state_namespace, status
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(instance_id) DO UPDATE SET
        card_id = excluded.card_id,
        version_id = excluded.version_id,
        surface_id = excluded.surface_id,
        x = excluded.x,
        y = excluded.y,
        width = excluded.width,
        height = excluded.height,
        state_namespace = excluded.state_namespace,
        status = excluded.status
      ''',
      [
        instance.instanceId,
        instance.cardId,
        instance.versionId,
        instance.surfaceId,
        instance.placement.x,
        instance.placement.y,
        instance.placement.width,
        instance.placement.height,
        instance.stateNamespace,
        instance.status.name,
      ],
    );
  }

  List<CardInstance> listInstances() {
    final rows = _connection.query(
      'SELECT * FROM card_instances ORDER BY instance_id',
    );
    return rows.map(_instanceFromRow).toList(growable: false);
  }

  void moveInstance({
    required String instanceId,
    required String surfaceId,
    required CardPlacement placement,
  }) {
    _connection.execute(
      '''
      UPDATE card_instances
      SET surface_id = ?, x = ?, y = ?, width = ?, height = ?
      WHERE instance_id = ?
      ''',
      [
        surfaceId,
        placement.x,
        placement.y,
        placement.width,
        placement.height,
        instanceId,
      ],
    );
  }

  void moveInstanceToSurface({
    required CardSurface surface,
    required String instanceId,
    required CardPlacement placement,
  }) {
    _connection.transaction(() {
      upsertSurface(surface);
      moveInstance(
        instanceId: instanceId,
        surfaceId: surface.id,
        placement: placement,
      );
    });
  }

  void switchInstanceVersion(String instanceId, String versionId) {
    _connection.execute(
      'UPDATE card_instances SET version_id = ? WHERE instance_id = ?',
      [versionId, instanceId],
    );
  }

  void putState(String namespace, String key, Object? value) {
    _connection.execute(
      '''
      INSERT INTO card_state (namespace, state_key, value_json)
      VALUES (?, ?, ?)
      ON CONFLICT(namespace, state_key) DO UPDATE SET
        value_json = excluded.value_json
      ''',
      [namespace, key, jsonEncode(value)],
    );
  }

  void replaceState(String namespace, Map<String, Object?> state) {
    _connection.execute('BEGIN IMMEDIATE');
    try {
      _connection.execute('DELETE FROM card_state WHERE namespace = ?', [
        namespace,
      ]);
      for (final entry in state.entries) {
        _connection.execute(
          '''
          INSERT INTO card_state (namespace, state_key, value_json)
          VALUES (?, ?, ?)
          ''',
          [namespace, entry.key, jsonEncode(entry.value)],
        );
      }
      _connection.execute('COMMIT');
    } catch (_) {
      _connection.execute('ROLLBACK');
      rethrow;
    }
  }

  Map<String, Object?> readState(String namespace) {
    final rows = _connection.query(
      '''
      SELECT state_key, value_json
      FROM card_state
      WHERE namespace = ?
      ORDER BY state_key
      ''',
      [namespace],
    );
    return {
      for (final row in rows)
        row['state_key']! as String: jsonDecode(row['value_json']! as String),
    };
  }

  void deleteState(String namespace, String key) {
    _connection.execute(
      'DELETE FROM card_state WHERE namespace = ? AND state_key = ?',
      [namespace, key],
    );
  }

  void upsertGrant(PermissionGrant grant) {
    final domains = grant.domains.toList()..sort();
    _connection.execute(
      '''
      INSERT INTO permission_grants (
        instance_id, version_id, capability, domains_json
      ) VALUES (?, ?, ?, ?)
      ON CONFLICT(instance_id, version_id, capability) DO UPDATE SET
        domains_json = excluded.domains_json
      ''',
      [
        grant.instanceId,
        grant.versionId,
        grant.capability,
        jsonEncode(domains),
      ],
    );
  }

  Set<PermissionGrant> grantsForInstance(String instanceId) {
    final rows = _connection.query(
      '''
      SELECT version_id, capability, domains_json
      FROM permission_grants
      WHERE instance_id = ?
      ORDER BY version_id, capability
      ''',
      [instanceId],
    );
    return rows.map((row) {
      final domains = (jsonDecode(row['domains_json']! as String) as List)
          .whereType<String>()
          .toSet();
      return PermissionGrant(
        instanceId: instanceId,
        versionId: row['version_id']! as String,
        capability: row['capability']! as String,
        domains: domains,
      );
    }).toSet();
  }

  void close() {
    _connection.close();
  }

  void _migrate() {
    _connection.execute('PRAGMA foreign_keys = ON');
    _connection.execute('PRAGMA journal_mode = WAL');
    _connection.execute('PRAGMA busy_timeout = 5000');
    if (schemaVersion < 1) {
      _connection.transaction(() {
        _connection.execute('''
          CREATE TABLE card_installations (
            version_id TEXT PRIMARY KEY,
            card_id TEXT NOT NULL,
            content_hash TEXT NOT NULL,
            runtime TEXT NOT NULL,
            installed_at TEXT NOT NULL,
            verified INTEGER NOT NULL
          )
        ''');
        _connection.execute('''
          CREATE TABLE surfaces (
            surface_id TEXT PRIMARY KEY,
            type TEXT NOT NULL,
            monitor_id TEXT,
            x REAL,
            y REAL,
            width REAL,
            height REAL,
            always_on_top INTEGER NOT NULL DEFAULT 0
          )
        ''');
        _connection.execute('''
          CREATE TABLE card_instances (
            instance_id TEXT PRIMARY KEY,
            card_id TEXT NOT NULL,
            version_id TEXT NOT NULL REFERENCES card_installations(version_id),
            surface_id TEXT NOT NULL REFERENCES surfaces(surface_id),
            x REAL NOT NULL,
            y REAL NOT NULL,
            width REAL NOT NULL,
            height REAL NOT NULL,
            state_namespace TEXT NOT NULL UNIQUE,
            status TEXT NOT NULL
          )
        ''');
        _connection.execute('''
          CREATE TABLE card_state (
            namespace TEXT NOT NULL,
            state_key TEXT NOT NULL,
            value_json TEXT NOT NULL,
            PRIMARY KEY(namespace, state_key)
          )
        ''');
        _connection.execute('''
          CREATE TABLE permission_grants (
            instance_id TEXT NOT NULL REFERENCES card_instances(instance_id)
              ON DELETE CASCADE,
            version_id TEXT NOT NULL,
            capability TEXT NOT NULL,
            domains_json TEXT NOT NULL,
            PRIMARY KEY(instance_id, version_id, capability)
          )
        ''');
        _connection.execute('PRAGMA user_version = 1');
      });
    }
    if (schemaVersion < 2) {
      _connection.transaction(() {
        _connection.execute(
          'ALTER TABLE card_installations ADD COLUMN definition_json TEXT',
        );
        _connection.execute(
          'ALTER TABLE card_installations ADD COLUMN key_id TEXT',
        );
        _connection.execute('PRAGMA user_version = 2');
      });
    }
    if (schemaVersion < 3) {
      _connection.transaction(() {
        _connection.execute(
          'ALTER TABLE surfaces ADD COLUMN last_focused_at TEXT',
        );
        _connection.execute('PRAGMA user_version = 3');
      });
    }
  }

  StoredInstallation _storedInstallationFromRow(Map<String, Object?> row) {
    final definitionJson =
        jsonDecode(row['definition_json']! as String) as Map<String, Object?>;
    return StoredInstallation(
      installation: CardInstallation(
        cardId: row['card_id']! as String,
        versionId: row['version_id']! as String,
        contentHash: row['content_hash']! as String,
        runtime: CardRuntime.values.byName(row['runtime']! as String),
        installedAt: DateTime.parse(row['installed_at']! as String).toUtc(),
        verified: (row['verified']! as int) != 0,
      ),
      definition: CardDefinition.fromJson(definitionJson),
      keyId: row['key_id']! as String,
    );
  }

  CardInstance _instanceFromRow(Map<String, Object?> row) {
    return CardInstance(
      instanceId: row['instance_id']! as String,
      cardId: row['card_id']! as String,
      versionId: row['version_id']! as String,
      surfaceId: row['surface_id']! as String,
      placement: CardPlacement(
        x: _double(row['x']),
        y: _double(row['y']),
        width: _double(row['width']),
        height: _double(row['height']),
      ),
      stateNamespace: row['state_namespace']! as String,
      status: CardInstanceStatus.values.byName(row['status']! as String),
    );
  }

  double _double(Object? value) {
    if (value is num) {
      return value.toDouble();
    }
    throw StateError('expected a numeric database value');
  }
}
