#!/bin/bash
set -e

mkdir -p /data

# Write config.yaml from environment variables
cat > /data/config.yaml << EOF
homeserver:
    address: https://matrix-conduit-production-8de3.up.railway.app
    domain: matrix-conduit-production-8de3.up.railway.app
    software: standard

appservice:
    address: https://meta-production-b153.up.railway.app
    hostname: 0.0.0.0
    port: 29319
    database:
        type: postgres
        uri: ${DATABASE_URL}
    id: meta
    bot:
        username: metabot
        displayname: Meta Bridge Bot
    as_token: d0cfd9178b845de71a554fe4390f8046e652793f4a954c69e0f12374bb056858
    hs_token: e8d4253f277fe3d07ed590a9092ef7bab0b8b890a76aea1a09d29803e99484b0

meta:
    mode: instagram

bridge:
    username_template: meta_{{.}}
    displayname_template: "{{or .DisplayName .Username}} (Instagram)"
    double_puppet_server_map: {}
    double_puppet_allow_discovery: false
    login_shared_secret_map: {}
    federate_rooms: false
    encryption:
        allow: false
        default: false
        require: false

logging:
    min_level: debug
    writers:
        - type: stdout
          format: pretty-colored
          min_level: debug
EOF

exec /docker-run.sh
