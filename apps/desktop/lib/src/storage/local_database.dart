import 'dart:convert';

import '../capabilities/capability.dart';
import '../cards/card_instance.dart';
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

  void upsertSurface(CardSurface surface) {
    final bounds = surface.bounds;
    _connection.execute(
      '''
      INSERT INTO surfaces (
        surface_id, type, monitor_id, x, y, width, height, always_on_top
      ) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
      ON CONFLICT(surface_id) DO UPDATE SET
        type = excluded.type,
        monitor_id = excluded.monitor_id,
        x = excluded.x,
        y = excluded.y,
        width = excluded.width,
        height = excluded.height,
        always_on_top = excluded.always_on_top
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
    if (schemaVersion >= 1) {
      return;
    }
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
