module go.mau.fi/mautrix-meta

go 1.26.0

toolchain go1.27.2

tool go.mau.fi/util/cmd/maubuild

require (
	github.com/apache/thrift v0.24.0
	github.com/beeper/poly1305 v0.0.0-20250815183548-d4eede7bbf3c
	github.com/bytemare/cryptotools v0.0.0-20210309005813-f9985b05742f
	github.com/bytemare/opaque v0.0.0-20210310201842-16a10d180656
	github.com/bytemare/voprf v0.0.0-20210221170502-f0af88d24d16
	github.com/coder/websocket v1.8.15
	github.com/gabriel-vasile/mimetype v1.4.15
	github.com/google/flatbuffers v25.12.19+incompatible
	github.com/google/go-querystring v1.2.0
	github.com/google/uuid v1.6.0
	github.com/gtank/ristretto255 v0.2.0
	github.com/imroc/req/v3 v3.56.0
	github.com/quic-go/quic-go v0.61.0
	github.com/refraction-networking/utls v1.8.2
	github.com/rs/zerolog v1.35.1
	github.com/tidwall/gjson v1.20.0
	github.com/zyedidia/clipboard v1.0.4
	go.mau.fi/libsignal v0.2.2
	go.mau.fi/util v0.11.0
	go.mau.fi/whatsmeow v0.0.0-20261014094007-f16f9ca30d6a
	golang.org/x/crypto v0.58.0
	golang.org/x/exp v0.0.0-20261014143641-f0251eb4aa3e
	golang.org/x/image v0.47.0
	golang.org/x/net v0.61.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
	maunium.net/go/mautrix v0.32.0
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/andybalholm/brotli v1.2.4 // indirect
	github.com/armfazh/h2c-go-ref v0.0.0-20210215173008-07e12a6f8e0d // indirect
	github.com/armfazh/tozan-ecc v0.1.3 // indirect
	github.com/beeper/argo-go v1.1.2 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.0 // indirect
	github.com/icholy/digest v1.2.0 // indirect
	github.com/klauspost/compress v1.20.0 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.52 // indirect
	github.com/petermattis/goid v0.0.0-20260918085751-abfca077860b // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	github.com/vmihailenco/msgpack/v5 v5.2.0 // indirect
	github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect
	github.com/yuin/goldmark v1.8.6 // indirect
	go.mau.fi/zeroconfig v0.2.0 // indirect
	golang.org/x/mod v0.42.0 // indirect
	golang.org/x/sync v0.24.0 // indirect
	golang.org/x/sys v0.49.0 // indirect
	golang.org/x/text v0.43.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	maunium.net/go/mauflag v1.0.0 // indirect
)

replace github.com/imroc/req/v3 => github.com/beeper/req/v3 v3.0.0-20260925135712-b49593750e56
