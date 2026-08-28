module go.mau.fi/mautrix-meta

go 1.26.0

toolchain go1.27.0

tool go.mau.fi/util/cmd/maubuild

require (
	github.com/apache/thrift v0.24.0
	github.com/beeper/poly1305 v0.0.0-20250815183548-d4eede7bbf3c
	github.com/coder/websocket v1.8.15
	github.com/gabriel-vasile/mimetype v1.4.15
	github.com/google/go-querystring v1.2.0
	github.com/google/uuid v1.6.0
	github.com/imroc/req/v3 v3.56.0
	github.com/refraction-networking/utls v1.8.2
	github.com/rs/zerolog v1.35.1
	github.com/tidwall/gjson v1.19.0
	github.com/zyedidia/clipboard v1.0.4
	go.mau.fi/libsignal v0.2.2
	go.mau.fi/util v0.10.1-0.20260820140024-eb612d936fde
	go.mau.fi/whatsmeow v0.0.0-20260816113502-fb386f152837
	golang.org/x/crypto v0.55.0
	golang.org/x/exp v0.0.0-20260813180055-c1d0aacb2297
	golang.org/x/image v0.45.0
	golang.org/x/net v0.58.0
	google.golang.org/protobuf v1.36.12
	gopkg.in/yaml.v3 v3.0.1
	maunium.net/go/mautrix v0.30.1-0.20260828211758-e9466a65f64c
)

require (
	filippo.io/edwards25519 v1.2.0 // indirect
	github.com/andybalholm/brotli v1.2.2 // indirect
	github.com/beeper/argo-go v1.1.2 // indirect
	github.com/coreos/go-systemd/v22 v22.7.0 // indirect
	github.com/elliotchance/orderedmap/v3 v3.1.0 // indirect
	github.com/icholy/digest v1.2.0 // indirect
	github.com/klauspost/compress v1.19.2 // indirect
	github.com/kr/text v0.2.0 // indirect
	github.com/lib/pq v1.12.3 // indirect
	github.com/mattn/go-colorable v0.1.15 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/mattn/go-sqlite3 v1.14.49 // indirect
	github.com/petermattis/goid v0.0.0-20260816044145-ed329add6b1b // indirect
	github.com/quic-go/qpack v0.6.0 // indirect
	github.com/quic-go/quic-go v0.61.0 // indirect
	github.com/rs/xid v1.6.0 // indirect
	github.com/skip2/go-qrcode v0.0.0-20200617195104-da1b6568686e // indirect
	github.com/tidwall/match v1.1.1 // indirect
	github.com/tidwall/pretty v1.2.1 // indirect
	github.com/tidwall/sjson v1.2.5 // indirect
	github.com/vektah/gqlparser/v2 v2.5.27 // indirect
	github.com/yuin/goldmark v1.8.5 // indirect
	go.mau.fi/zeroconfig v0.2.0 // indirect
	golang.org/x/mod v0.40.0 // indirect
	golang.org/x/sync v0.22.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/text v0.41.0 // indirect
	gopkg.in/natefinch/lumberjack.v2 v2.2.1 // indirect
	maunium.net/go/mauflag v1.0.0 // indirect
)

replace github.com/imroc/req/v3 => github.com/beeper/req/v3 v3.0.0-20260808092153-100cef0a2fbd
