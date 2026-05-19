#!/bin/sh
set -e

DATA_DIR=/data
CONFIG="$DATA_DIR/config.yaml"

mkdir -p "$DATA_DIR"

# Always write fresh config from env vars
cat > "$CONFIG" << YAML
homeserver:
    address: ${MATRIX_HOMESERVER_URL}
    domain: ${MATRIX_HOMESERVER_DOMAIN}
    software: standard

appservice:
    address: ${MAUTRIX_PUBLIC_URL}
    hostname: 0.0.0.0
    port: 29319
    database:
        type: postgres
        uri: ${DATABASE_URL}
    id: meta
    bot:
        username: metabot
        displayname: Meta Bridge Bot
        avatar: mxc://maunium.net/ygtkteZsXnGJLJHRchUwYWak
    as_token: ${MAUTRIX_AS_TOKEN}
    hs_token: ${MAUTRIX_HS_TOKEN}

meta:
    mode: instagram

bridge:
    username_template: meta_{{.}}
    displayname_template: "{{or .DisplayName .Username}} (Instagram)"
    double_puppet_server_map: {}
    double_puppet_allow_discovery: false
    login_shared_secret_map: {}
    private_chat_portal_meta: default
    portal_message_buffer: 128
    federate_rooms: false
    encryption:
        allow: false
        default: false
        require: false
    relay:
        enabled: false

logging:
    min_level: debug
    writers:
        - type: stdout
          format: pretty-colored
          min_level: debug
YAML
echo "[entrypoint] Config written to $CONFIG"

exec /usr/bin/mautrix-meta -c "$CONFIG" "$@"
