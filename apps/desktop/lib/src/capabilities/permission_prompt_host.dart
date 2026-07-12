import 'package:flutter/material.dart';

import 'permission_request_controller.dart';

class PermissionPromptHost extends StatelessWidget {
  const PermissionPromptHost({
    required this.controller,
    required this.child,
    super.key,
  });

  final PermissionRequestController controller;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, _) {
        final request = controller.current;
        return Stack(
          children: [
            child,
            if (request != null) ...[
              const Positioned.fill(
                child: ModalBarrier(
                  dismissible: false,
                  color: Color(0x73000000),
                ),
              ),
              Center(
                child: Card(
                  child: ConstrainedBox(
                    constraints: const BoxConstraints(maxWidth: 440),
                    child: Padding(
                      padding: const EdgeInsets.all(24),
                      child: Column(
                        mainAxisSize: MainAxisSize.min,
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          const Text(
                            '卡片请求系统能力',
                            style: TextStyle(
                              fontSize: 20,
                              fontWeight: FontWeight.w700,
                            ),
                          ),
                          const SizedBox(height: 14),
                          Text('${request.cardId} 请求 ${request.capability}'),
                          if (request.domain case final domain?) ...[
                            const SizedBox(height: 8),
                            Text('仅授权域名：$domain'),
                          ],
                          const SizedBox(height: 22),
                          Row(
                            mainAxisAlignment: MainAxisAlignment.end,
                            children: [
                              TextButton(
                                key: const Key('deny-capability'),
                                onPressed: controller.deny,
                                child: const Text('拒绝'),
                              ),
                              const SizedBox(width: 10),
                              FilledButton(
                                key: const Key('approve-capability'),
                                onPressed: controller.approve,
                                child: const Text('允许'),
                              ),
                            ],
                          ),
                        ],
                      ),
                    ),
                  ),
                ),
              ),
            ],
          ],
        );
      },
    );
  }
}
