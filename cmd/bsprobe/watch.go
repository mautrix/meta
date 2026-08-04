package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/coder/websocket"
)

// watch-events attaches to the realtime endpoint discovered by `bsprobe har`
// and classifies frames as they arrive. This is the live confirmation of the
// Case A / Case B call: HAR cannot decode binary frames, so a capture can only
// suggest MQTT — this proves it.

// mqttPacketName maps the high nibble of an MQTT fixed header to its control
// packet type. Recognising these in the first byte is strong evidence that
// Business Suite rides the same MQTT transport pkg/messagix/socket speaks.
func mqttPacketName(b byte) string {
	switch b >> 4 {
	case 1:
		return "CONNECT"
	case 2:
		return "CONNACK"
	case 3:
		return "PUBLISH"
	case 4:
		return "PUBACK"
	case 8:
		return "SUBSCRIBE"
	case 9:
		return "SUBACK"
	case 12:
		return "PINGREQ"
	case 13:
		return "PINGRESP"
	case 14:
		return "DISCONNECT"
	default:
		return ""
	}
}

type frameStats struct {
	Text        int
	Binary      int
	MQTTLike    int
	Lightspeed  int
	GraphQLSub  int
	MarkerCount map[string]int
}

func cmdWatchEvents(args []string) error {
	fs := flag.NewFlagSet("watch-events", flag.ExitOnError)
	sessionPath := fs.String("session", defaultSessionFile, "session file")
	wsURL := fs.String("url", "", "realtime endpoint (wss://...) as reported by 'bsprobe har'")
	duration := fs.Duration("for", 2*time.Minute, "how long to listen")
	dump := fs.String("dump-frames", "", "append redacted frames to this file")
	maxFrames := fs.Int("max", 200, "stop after this many frames")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *wsURL == "" {
		return fmt.Errorf("--url is required\n\n" +
			"Run 'bsprobe har <capture.har>' first; it prints the realtime endpoints it saw.\n" +
			"There is no default here on purpose - guessing the endpoint is exactly the\n" +
			"kind of assumption this harness exists to avoid.")
	}

	s, err := LoadSession(*sessionPath)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), *duration)
	defer cancel()
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	header := http.Header{}
	header.Set("User-Agent", s.UserAgent)
	header.Set("Origin", businessHost)
	var cookiePairs []string
	for _, c := range s.Cookies {
		cookiePairs = append(cookiePairs, c.Name+"="+c.Value)
	}
	header.Set("Cookie", strings.Join(cookiePairs, "; "))

	fmt.Printf("dialing %s\n", *wsURL)
	conn, resp, err := websocket.Dial(ctx, *wsURL, &websocket.DialOptions{
		HTTPHeader: header,
	})
	if err != nil {
		if resp != nil {
			return fmt.Errorf("dial failed (HTTP %d): %w", resp.StatusCode, err)
		}
		return fmt.Errorf("dial failed: %w", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")
	conn.SetReadLimit(8 << 20)
	fmt.Printf("connected, listening for %s (ctrl-c to stop)\n\n", *duration)

	var dumpFile *os.File
	if *dump != "" {
		if err := os.MkdirAll(filepath.Dir(*dump), 0o755); err != nil {
			return err
		}
		dumpFile, err = os.OpenFile(*dump, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		defer dumpFile.Close()
	}

	stats := &frameStats{MarkerCount: map[string]int{}}
	for i := 0; i < *maxFrames; i++ {
		typ, data, err := conn.Read(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			return fmt.Errorf("read frame %d: %w", i, err)
		}
		classifyFrame(stats, typ, data, i, dumpFile)
	}

	fmt.Print(renderFrameVerdict(stats))
	return nil
}

func classifyFrame(stats *frameStats, typ websocket.MessageType, data []byte, idx int, dumpFile *os.File) {
	kind := "text"
	if typ == websocket.MessageBinary {
		kind = "binary"
		stats.Binary++
	} else {
		stats.Text++
	}

	note := ""
	if typ == websocket.MessageBinary && len(data) > 0 {
		if name := mqttPacketName(data[0]); name != "" {
			stats.MQTTLike++
			note = "mqtt:" + name
		}
	}

	body := string(data)
	for _, marker := range lightspeedMarkers {
		if strings.Contains(body, marker) {
			stats.MarkerCount[marker]++
		}
	}
	if len(stats.MarkerCount) >= 2 {
		stats.Lightspeed++
	}
	if strings.Contains(body, `"topic"`) && strings.Contains(body, `"subscription`) {
		stats.GraphQLSub++
		if note == "" {
			note = "graphql-subscription-like"
		}
	}

	preview := RedactString(body)
	if len(preview) > 200 {
		preview = preview[:200] + "..."
	}
	preview = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' {
			return '.'
		}
		return r
	}, preview)

	fmt.Printf("[%03d] %-6s %6dB %-28s %s\n", idx, kind, len(data), note, preview)

	if dumpFile != nil {
		rec := map[string]any{
			"idx":     idx,
			"kind":    kind,
			"bytes":   len(data),
			"note":    note,
			"preview": preview,
		}
		b, _ := json.Marshal(rec)
		fmt.Fprintln(dumpFile, string(b))
	}
}

func renderFrameVerdict(stats *frameStats) string {
	var b strings.Builder
	fmt.Fprintf(&b, "\n---\nframes: text=%d binary=%d mqtt-like=%d\n", stats.Text, stats.Binary, stats.MQTTLike)
	if len(stats.MarkerCount) > 0 {
		fmt.Fprintf(&b, "lightspeed markers:\n")
		for m, n := range stats.MarkerCount {
			fmt.Fprintf(&b, "  %-18s %d\n", m, n)
		}
	}

	switch {
	case stats.MQTTLike > 0 && len(stats.MarkerCount) >= 2:
		fmt.Fprintf(&b, "\nVERDICT: CASE A - MQTT transport carrying Lightspeed payloads.\n")
		fmt.Fprintf(&b, "Reuse pkg/messagix socket, lightspeed, table and syncManager. Add Business\n")
		fmt.Fprintf(&b, "Suite app config, sync groups and mailbox identifiers only.\n")
	case stats.MQTTLike > 0:
		fmt.Fprintf(&b, "\nVERDICT: CASE A LIKELY - MQTT control packets seen, payloads not yet decoded.\n")
		fmt.Fprintf(&b, "Next: feed a captured PUBLISH payload through pkg/messagix/lightspeed and see\n")
		fmt.Fprintf(&b, "whether it decodes into an LSTable. That is the real confirmation.\n")
	case stats.GraphQLSub > 0:
		fmt.Fprintf(&b, "\nVERDICT: CASE B - GraphQL-subscription-shaped frames, not Lightspeed.\n")
		fmt.Fprintf(&b, "Build pkg/bizsuite/realtime/ with its own decoder. Do not bend messagix.\n")
	case stats.Text+stats.Binary == 0:
		fmt.Fprintf(&b, "\nVERDICT: INCONCLUSIVE - no frames received.\n")
		fmt.Fprintf(&b, "The endpoint may need a subscribe handshake before it sends anything.\n")
	default:
		fmt.Fprintf(&b, "\nVERDICT: INCONCLUSIVE - frames received but unrecognised.\n")
		fmt.Fprintf(&b, "Inspect the dump and extend lightspeedMarkers in har.go.\n")
	}
	return b.String()
}
