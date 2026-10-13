# mautrix-meta
A Matrix-Facebook Messenger and Instagram DM puppeting bridge.

This repo contains two bridges: mautrix-meta for Facebook Messenger and
mautrix-instagram for Instagram DMs. When setting up the Instagram bridge,
note the differences in setup instructions (different build script, different
docker tags, etc).

## Documentation
All setup and usage instructions are located on [docs.mau.fi]. Some quick links:

[docs.mau.fi]: https://docs.mau.fi/bridges/go/meta/index.html

* Facebook [Bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=meta)
  (or [with Docker](https://docs.mau.fi/bridges/general/docker-setup.html?bridge=meta))
* Instagram [Bridge setup](https://docs.mau.fi/bridges/go/setup.html?bridge=instagram)
  (or [with Docker](https://docs.mau.fi/bridges/general/docker-setup.html?bridge=instagram))
* Basic usage: [Authentication](https://docs.mau.fi/bridges/go/meta/authentication.html)

### Building from source
Both bridges also require the native library for Messenger history recovery.
With Rust 1.98.1 installed, build it and set the library path before building the bridge:

```sh
cargo build --locked --release --manifest-path pkg/messagix/securehistory/opaque/Cargo.toml
export CGO_LDFLAGS="${CGO_LDFLAGS:-} -L$(pwd)/pkg/messagix/securehistory/opaque/target/release"
./build-fb.sh
```

Use `./build-ig.sh` for Instagram. The Dockerfiles build the native library automatically.

### Features & Roadmap
[ROADMAP.md](ROADMAP.md) contains a general overview of what is supported by the bridge.

## Discussion
Matrix room: [#meta:maunium.net](https://matrix.to/#/#meta:maunium.net)
