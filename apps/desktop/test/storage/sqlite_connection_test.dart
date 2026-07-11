import 'package:agent_card_desktop/src/storage/sqlite_connection.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('executes prepared statements against an in-memory SQLite database', () {
    final database = SqliteConnection.openInMemory();
    addTearDown(database.close);

    database.execute(
      'CREATE TABLE cards (id TEXT PRIMARY KEY, count INTEGER, ratio REAL)',
    );
    database.execute('INSERT INTO cards (id, count, ratio) VALUES (?, ?, ?)', [
      'card-1',
      3,
      0.5,
    ]);

    final rows = database.query(
      'SELECT id, count, ratio FROM cards WHERE id = ?',
      ['card-1'],
    );

    expect(rows, [
      {'id': 'card-1', 'count': 3, 'ratio': 0.5},
    ]);
  });

  test('rolls back failed transactions', () {
    final database = SqliteConnection.openInMemory();
    addTearDown(database.close);
    database.execute('CREATE TABLE values_table (value INTEGER)');

    expect(
      () => database.transaction(() {
        database.execute('INSERT INTO values_table (value) VALUES (?)', [1]);
        throw StateError('stop');
      }),
      throwsStateError,
    );

    expect(database.query('SELECT value FROM values_table'), isEmpty);
  });
}
