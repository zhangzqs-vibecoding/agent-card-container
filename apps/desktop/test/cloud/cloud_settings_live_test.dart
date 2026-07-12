import 'dart:io';

import 'package:agent_card_desktop/src/cloud/cloud_api_client.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  final enabled = Platform.environment['AGENTCARD_CLOUD_SETTINGS_LIVE'] == '1';

  test(
    'connects to a live Agent Card Cloud health and authenticated catalog',
    () async {
      final url = Platform.environment['AGENTCARD_CLOUD_URL'];
      final token = Platform.environment['AGENTCARD_ACCESS_TOKEN'];
      expect(url, isNotEmpty);
      expect(token, isNotEmpty);
      final client = CloudApiClient(
        baseUri: Uri.parse(url!),
        tokenProvider: () async => token!,
        allowInsecureForDevelopment: true,
      );
      addTearDown(client.close);

      await client.testConnection();
    },
    skip: enabled ? false : 'requires explicit live test environment',
  );
}
