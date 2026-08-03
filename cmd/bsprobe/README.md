# bsprobe

Protocol discovery harness for the Meta Business Suite inbox.

It exists to answer one question before any connector code is written:

> Does `business.facebook.com` speak the same Lightspeed protocol that
> `pkg/messagix` already implements, or something else?

**Case A** (Lightspeed over MQTT) means the new transport reuses
`pkg/messagix/{socket,lightspeed,table,syncManager.go}` and the work is mostly
configuration. **Case B** means a separate decoder under `pkg/bizsuite/realtime/`
and a much larger job. Committing to the wrong one costs weeks, which is why
this tool runs first.

This is a research instrument. Nothing here is imported by `pkg/connector`, and
it must stay that way.

## Why nothing is hard-coded

Meta rotates persisted GraphQL `doc_id` values. A table of them compiled into Go
works for days and then collapses silently. So:

- `bsprobe har` generates `fixtures/operations.json` from **your** capture.
- `fixtures/roles.json` maps a semantic role (`list_threads`) onto whichever
  `friendly_name` that capture revealed.
- When calls start failing, you re-record and re-run `har`. You do not edit Go.

## Capture procedure

Use a dedicated test Page and your own Facebook account. Open DevTools **before**
loading the inbox, enable *Preserve log*, then perform only these actions:

1. Open Business Suite Inbox
2. Select the Page
3. Load the conversation list
4. Open one conversation
5. Receive a test message
6. Send a text reply
7. Mark unread, then read
8. Load older messages

Right-click the Network panel → *Save all as HAR with content*.

Chrome only records WebSocket frames for connections opened while DevTools is
already recording. If `bsprobe har` reports no realtime traffic, that is almost
always why.

## Run order

```sh
go build ./cmd/bsprobe

# 1. Confirm the pasted session authenticates.
#    Save a DevTools "Copy as cURL" blob to .bsprobe/session.curl first.
./bsprobe validate-session

# 2. Offline analysis. Produces fixtures/operations.json + fixtures/shapes/,
#    and prints the Case A / Case B verdict.
./bsprobe har --out fixtures capture.har

# 3. Map the roles the connector needs, using the friendly names from step 2.
$EDITOR fixtures/roles.json

# 4. Extract runtime tokens and the business/asset/mailbox identifiers.
./bsprobe bootstrap --save fixtures/bootstrap.json

# 5. Confirm the realtime verdict live. The --url comes from step 2's report;
#    there is deliberately no default.
./bsprobe watch-events --url wss://... --dump-frames fixtures/frames.jsonl

# 6. Exercise the captured operations.
./bsprobe list-threads --mailbox <id>
./bsprobe call --op <FriendlyName> --vars '{"thread_id":"..."}'
```

## Redaction

Everything written to disk passes through `redact.go` at capture time — a
fixture that was never written unredacted cannot leak.

- **Secrets** (`fb_dtsg`, `lsd`, `jazoest`, cookies, tokens) → removed entirely.
- **Content** (message text, customer names, emails, phones) → replaced with a
  length hint.
- **Protocol facts** (`friendly_name`, `doc_id`, variable *keys*, the routing
  identifiers) → preserved, because they are the entire point.
- URL query strings are stripped; host and path survive.

`--shape` (the default on live commands) goes further and emits keys and types
with every value dropped. Prefer shapes for anything you intend to commit.

`.gitignore` in this directory excludes sessions, HARs and raw HTML. Review
`fixtures/` before committing anyway.

## Tests

```sh
go test ./cmd/bsprobe
```

The redaction tests are the important ones: they assert that a live token never
survives serialisation, and that `friendly_name` / `doc_id` are *not* destroyed
by over-eager redaction. The HAR tests pin the Case A / Case B / inconclusive
verdict logic against synthetic captures.

## What this does not do

It does not send production traffic, automate a browser, or touch the bridge.
It reads a session you paste and a capture you record.

Note the standing question this work sits on top of: for Page inboxes Meta also
offers the official Messenger Platform API (Conversations API + webhooks + Send
API), which is supported and versioned. It carries a 24-hour outbound messaging
window and needs app review to serve Pages you do not own. If those limits are
acceptable, that route needs none of this.
