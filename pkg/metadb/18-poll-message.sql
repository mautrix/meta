-- v18 (compatible with v11+): Index Messenger poll messages
CREATE TABLE meta_poll (
    bridge_id       TEXT   NOT NULL,
    portal_id       TEXT   NOT NULL,
    poll_id         BIGINT NOT NULL,
    portal_receiver TEXT   NOT NULL,
    message_rowid   BIGINT NOT NULL,

    PRIMARY KEY (bridge_id, portal_id, poll_id, portal_receiver),
    CONSTRAINT meta_poll_portal_fkey FOREIGN KEY (bridge_id, portal_id, portal_receiver)
        REFERENCES portal (bridge_id, id, receiver) ON DELETE CASCADE ON UPDATE CASCADE,
    CONSTRAINT meta_poll_message_fkey FOREIGN KEY (message_rowid)
        REFERENCES message (rowid) ON DELETE CASCADE ON UPDATE CASCADE
);
CREATE INDEX meta_poll_message_idx ON meta_poll (message_rowid);
