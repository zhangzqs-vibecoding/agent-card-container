BEGIN;

UPDATE generation_sessions AS session
SET confirmed_requirement_json = jsonb_build_object(
  'initialPrompt', session.prompt,
  'additionalMessages', COALESCE((
    SELECT jsonb_agg(
      jsonb_build_object(
        'role', message.role,
        'content', message.content,
        'createdAt', message.created_at
      )
      ORDER BY message.sequence
    )
    FROM generation_messages AS message
    WHERE message.session_id = session.id
      AND message.role = 'user'
      AND message.sequence > (
        SELECT min(initial_message.sequence)
        FROM generation_messages AS initial_message
        WHERE initial_message.session_id = session.id
      )
  ), '[]'::jsonb),
  'target', session.target,
  'locale', session.locale,
  'allowedCapabilities', jsonb_build_array('storage', 'window.manageSelf'),
  'confirmedAt', COALESCE((
    SELECT min(event.created_at)
    FROM generation_events AS event
    WHERE event.session_id = session.id
      AND event.stage = 'queued'
  ), session.updated_at)
)
WHERE session.status IN ('queued', 'generating', 'validating', 'ready', 'failed')
   OR (
     session.status = 'cancelled'
     AND EXISTS (
       SELECT 1
       FROM generation_events AS event
       WHERE event.session_id = session.id
         AND event.stage = 'queued'
     )
   );

COMMIT;
