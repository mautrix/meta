-- v0 -> v1 (compatible with v1+): Latest schema
CREATE TABLE meta_thread (
    parent_key BIGINT NOT NULL,
    thread_key BIGINT NOT NULL,
    message_id TEXT   NOT NULL,

    PRIMARY KEY (thread_key),
    CONSTRAINT meta_thread_message_id_unique UNIQUE (message_id)
);
