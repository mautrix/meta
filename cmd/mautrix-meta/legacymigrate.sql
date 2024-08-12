INSERT INTO "user" (bridge_id, mxid, management_room, access_token)
SELECT '', mxid, management_room, ''
FROM user_old;

INSERT INTO user_login (bridge_id, user_mxid, id, remote_name, space_room, metadata, remote_profile)
SELECT
    '', -- bridge_id
    mxid, -- user_mxid
    CAST(meta_id AS TEXT), -- id
    '', -- remote_name
    space_room,
    -- only: postgres
    jsonb_build_object
-- only: sqlite (line commented)
--  json_object
    (
        'platform', 'hacky platform placeholder',
        'cookies', json(cookies),
        'wa_device_id', wa_device_id
    ),
    '{}'
FROM user_old WHERE meta_id IS NOT NULL;

INSERT INTO ghost (
    bridge_id, id, name, avatar_id, avatar_hash, avatar_mxc,
    name_set, avatar_set, contact_info_set, is_bot, identifiers, metadata
)
SELECT
    '', -- bridge_id
    CAST(id AS TEXT), -- id
    name, -- name
    avatar_id, -- avatar_id
    '', -- avatar_hash
    avatar_url, -- avatar_mxc
    name_set, -- name_set
    avatar_set, -- avatar_set
    contact_info_set, -- contact_info_set
    false, -- is_bot
    '[]', -- identifiers
    '{}' -- metadata
FROM puppet_old;

INSERT INTO portal (
    bridge_id, id, receiver, mxid, relay_bridge_id, relay_login_id, other_user_id,
    name, topic, avatar_id, avatar_hash, avatar_mxc, name_set, avatar_set, topic_set, name_is_custom,
    in_space, room_type, metadata
)
SELECT
    '', -- bridge_id
    CAST(thread_id AS TEXT), -- id
    CASE WHEN receiver<>0 THEN CAST(receiver AS TEXT) ELSE '' END, -- receiver
    mxid,
    CASE WHEN portal_old.relay_user_id<>'' THEN '' END, -- relay_bridge_id
    CASE WHEN portal_old.relay_user_id<>'' THEN (SELECT id FROM user_login WHERE user_mxid=portal_old.relay_user_id) END, -- relay_login_id
    CASE WHEN thread_type IN (1, 15) THEN CAST(thread_id AS TEXT) END, -- other_user_id
    name,
    '', -- topic
    avatar_id,
    '', -- avatar_hash
    avatar_url, -- avatar_mxc
    name_set,
    avatar_set,
    false, -- topic_set
    false, -- name_is_custom
    false, -- in_space
    CASE WHEN thread_type in (1, 15) THEN 'dm' ELSE '' END, -- room_type
    -- only: postgres
    jsonb_build_object
-- only: sqlite (line commented)
--  json_object
    (
        'thread_type', thread_type,
        'whatsapp_server', whatsapp_server
    ) -- metadata
FROM portal_old;

INSERT INTO user_portal (bridge_id, user_mxid, login_id, portal_id, portal_receiver, in_space, preferred, last_read)
SELECT
    '', -- bridge_id
    user_mxid,
    (SELECT id FROM user_login WHERE user_mxid=user_portal_old.user_mxid), -- login_id
    CAST(portal_thread_id AS TEXT), -- portal_id,
    CASE WHEN portal_receiver<>0 THEN CAST(portal_receiver AS TEXT) ELSE '' END, -- portal_receiver
    in_space,
    false, -- preferred
    last_read_ts * 1000000
FROM user_portal_old;

ALTER TABLE message_old ADD COLUMN new_id TEXT;

UPDATE message_old SET new_id = CASE
    WHEN id LIKE 'mid.%' THEN ('fb:' || id)
    ELSE ('wa'
        || ':' || CAST(thread_id AS TEXT) || '@' || (SELECT whatsapp_server FROM portal_old WHERE portal_old.thread_id=message_old.thread_id AND receiver=thread_receiver)
        || ':' || CAST(msg_sender AS TEXT) || '@msgr'
        || ':' || id)
END;

INSERT INTO message (
    bridge_id, id, part_id, mxid, room_id, room_receiver, sender_id, sender_mxid, timestamp, edit_count, metadata
)
SELECT
    '', -- bridge_id
    new_id, -- id
    CASE WHEN part_index=0 THEN '' ELSE CAST(part_index AS TEXT) END, -- part_id
    mxid,
    CAST(thread_id AS TEXT), -- room_id
    CASE WHEN thread_receiver<>0 THEN CAST(thread_receiver AS TEXT) ELSE '' END, -- room_receiver
    CAST(msg_sender AS TEXT), -- sender_id
    '', -- sender_mxid
    timestamp * 1000000, -- timestamp
    edit_count, -- edit_count
    '{}' -- metadata
FROM message_old;

INSERT INTO reaction (
    bridge_id, message_id, message_part_id, sender_id, emoji_id, room_id, room_receiver, mxid, timestamp, emoji, metadata
)
SELECT
    '', -- bridge_id
    (SELECT new_id FROM message_old WHERE id=message_id AND part_index=0 AND message_old.thread_receiver=reaction_old.thread_receiver), -- message_id
    '', -- message_part_id
    CAST(reaction_sender AS TEXT), -- sender_id
    '', -- emoji_id
    CAST(thread_id AS TEXT), -- room_id
    CASE WHEN thread_receiver<>0 THEN CAST(thread_receiver AS TEXT) ELSE '' END, -- room_receiver
    mxid,
    (SELECT (timestamp * 1000000) + 1 FROM message_old WHERE id=message_id AND part_index=0 AND message_old.thread_receiver=reaction_old.thread_receiver), -- timestamp
    emoji, -- emoji
    '{}' -- metadata
FROM reaction_old;

INSERT INTO backfill_task (
    bridge_id, portal_id, portal_receiver, user_login_id, batch_count, is_done,
    cursor, oldest_message_id, dispatched_at, completed_at, next_dispatch_min_ts
)
SELECT
    '', -- bridge_id
    CAST(portal_id AS TEXT), -- portal_id
    CASE WHEN portal_receiver<>0 THEN CAST(portal_receiver AS TEXT) ELSE '' END, -- portal_receiver
    (SELECT id FROM user_login WHERE user_mxid=backfill_task_old.user_mxid), -- user_login_id
    page_count, -- batch_count
    finished, -- is_done
    '', -- cursor
    '', -- oldest_message_id
    dispatched_at * 1000000, -- dispatched_at
    completed_at * 1000000, -- completed_at
    cooldown_until * 1000000 -- next_dispatch_min_ts
FROM backfill_task_old
WHERE true
ON CONFLICT DO NOTHING;

DROP TABLE reaction_old;
DROP TABLE message_old;
DROP TABLE backfill_task_old;
DROP TABLE user_portal_old;
DROP TABLE portal_old;
DROP TABLE puppet_old;
DROP TABLE user_old;
