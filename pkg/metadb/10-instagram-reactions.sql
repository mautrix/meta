-- v10 (compatible with v8+): Add table for Instagram reaction mappings

CREATE TABLE meta_instagram_reaction (
    bridge_id           TEXT   NOT NULL,
    portal_id           TEXT   NOT NULL,
    portal_receiver     TEXT   NOT NULL,
    target_message_id   TEXT   NOT NULL,
    reaction_sender     BIGINT NOT NULL,
    reaction_message_id TEXT   NOT NULL,

    PRIMARY KEY (bridge_id, portal_receiver, reaction_message_id),
    CONSTRAINT ig_reaction_portal_fkey FOREIGN KEY (bridge_id, portal_id, portal_receiver)
        REFERENCES portal (bridge_id, id, receiver) ON DELETE CASCADE ON UPDATE CASCADE
);
