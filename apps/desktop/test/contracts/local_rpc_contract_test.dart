import 'dart:convert';
import 'dart:io';

import 'package:agent_card_desktop/src/contracts/local_rpc_contract.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  test('Dart local RPC names match the shared contract', () {
    final contract =
        jsonDecode(
              File(
                '../../contracts/local-rpc/contract.json',
              ).readAsStringSync(),
            )
            as Map<String, Object?>;

    expect(contract['contractVersion'], LocalRpcContract.version);
    expect((contract['methods']! as List).toSet(), LocalRpcContract.methods);
    expect((contract['events']! as List).toSet(), LocalRpcContract.events);
    expect(
      (contract['stableErrors']! as List).toSet(),
      LocalRpcContract.stableErrors,
    );
  });
}
