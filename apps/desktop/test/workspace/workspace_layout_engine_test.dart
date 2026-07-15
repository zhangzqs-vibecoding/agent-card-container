import 'package:agent_card_desktop/src/surfaces/surface.dart';
import 'package:agent_card_desktop/src/workspace/workspace_layout_engine.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  const engine = WorkspaceLayoutEngine();

  group('WorkspaceLayoutEngine.resolve', () {
    test('snaps and constrains a candidate to the twelve-column grid', () {
      final placement = engine.resolve(
        const CardPlacement(x: 10.6, y: -2.2, width: 4.4, height: 1.2),
      );

      expect(placement.x, 8);
      expect(placement.y, 0);
      expect(placement.width, 4);
      expect(placement.height, 2);
    });

    test('rejects non-finite and non-positive placements', () {
      expect(
        () => engine.resolve(
          const CardPlacement(x: double.nan, y: 0, width: 4, height: 3),
        ),
        throwsFormatException,
      );
      expect(
        () => engine.resolve(
          const CardPlacement(x: 0, y: 0, width: 0, height: 3),
        ),
        throwsFormatException,
      );
    });

    test('allows touching edges but moves intersections down by rows', () {
      final placement = engine.resolve(
        const CardPlacement(x: 3, y: 0, width: 3, height: 2),
        occupied: const [
          WorkspaceLayoutItem(
            instanceId: 'left',
            placement: CardPlacement(x: 0, y: 0, width: 3, height: 2),
          ),
          WorkspaceLayoutItem(
            instanceId: 'blocking',
            placement: CardPlacement(x: 3, y: 1, width: 3, height: 2),
          ),
        ],
      );

      expect(placement.x, 3);
      expect(placement.y, 3);
    });

    test('ignores the edited instance during collision detection', () {
      final placement = engine.resolve(
        const CardPlacement(x: 1, y: 1, width: 4, height: 3),
        occupied: const [
          WorkspaceLayoutItem(
            instanceId: 'editing',
            placement: CardPlacement(x: 1, y: 1, width: 4, height: 3),
          ),
        ],
        ignoreInstanceId: 'editing',
      );

      expect(placement.y, 1);
    });
  });

  test('autoPlace scans rows and columns for the first free position', () {
    final placement = engine.autoPlace(
      width: 4,
      height: 2,
      occupied: const [
        WorkspaceLayoutItem(
          instanceId: 'first',
          placement: CardPlacement(x: 0, y: 0, width: 4, height: 2),
        ),
        WorkspaceLayoutItem(
          instanceId: 'second',
          placement: CardPlacement(x: 4, y: 0, width: 4, height: 2),
        ),
      ],
    );

    expect(placement.x, 8);
    expect(placement.y, 0);
    expect(placement.width, 4);
    expect(placement.height, 2);
  });
}
