// Command bsprobe is a protocol discovery harness for the Meta Business Suite
// inbox.
//
// It exists to answer one question before any connector code is written: does
// business.facebook.com speak the same Lightspeed protocol that pkg/messagix
// already implements (Case A, reuse it), or something else (Case B, build a
// separate transport)? Guessing that answer wrong costs weeks.
//
// This is a research instrument, not part of the bridge. Nothing here should
// ever be imported by pkg/connector. Every artifact it writes is redacted at
// capture time.
//
// Typical run:
//
//	bsprobe validate-session
//	bsprobe har --out fixtures capture.har     # offline; produces operations.json
//	bsprobe bootstrap --save fixtures/bootstrap.json
//	bsprobe watch-events --url wss://...       # url comes from the har report
//	bsprobe list-threads --mailbox <id>
package main

import (
	"fmt"
	"os"
)

const usage = `bsprobe - Meta Business Suite protocol discovery harness

  validate-session   check that the pasted session authenticates against the
                     Business Suite inbox
  har <file.har>     analyse a DevTools recording offline: extract persisted
                     GraphQL operations, find the realtime endpoint, and decide
                     Case A (Lightspeed) vs Case B (something else)
  bootstrap          fetch the inbox document and extract fb_dtsg, lsd, jazoest,
                     revision and the business/asset/mailbox identifiers
  messagix           load a Page inbox through the production messagix decoder
  watch-events       attach to the realtime endpoint and classify live frames
  list-assets        call the captured "list_assets" operation
  list-threads       call the captured "list_threads" operation
  send-test          call the captured "send_text" operation
  call --op NAME     invoke any captured operation with arbitrary variables

Flags are per-command; run 'bsprobe <command> -h'.

Order matters: har first (it writes fixtures/operations.json), then map roles in
fixtures/roles.json, then the live commands. No doc_id is ever hard-coded — when
Meta rotates them, re-record and re-run har.
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}

	cmd := os.Args[1]
	args := os.Args[2:]

	var err error
	switch cmd {
	case "validate-session":
		err = cmdValidateSession(args)
	case "har":
		err = cmdHAR(args)
	case "bootstrap":
		err = cmdBootstrap(args)
	case "messagix":
		err = cmdMessagix(args)
	case "watch-events":
		err = cmdWatchEvents(args)
	case "list-assets":
		err = cmdListAssets(args)
	case "list-threads":
		err = cmdListThreads(args)
	case "send-test":
		err = cmdSendTest(args)
	case "call":
		err = cmdCall(args)
	case "help", "-h", "--help":
		fmt.Print(usage)
		return
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n\n%s", cmd, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "\nbsprobe %s: %v\n", cmd, err)
		os.Exit(1)
	}
}
