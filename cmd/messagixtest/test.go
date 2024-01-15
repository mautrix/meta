package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rs/zerolog"
	"go.mau.fi/util/exerrors"

	"go.mau.fi/mautrix-meta/messagix"
	"go.mau.fi/mautrix-meta/messagix/cookies"
	"go.mau.fi/mautrix-meta/messagix/types"
)

var log = zerolog.New(zerolog.NewConsoleWriter(func(w *zerolog.ConsoleWriter) {
	w.TimeFormat = time.RFC3339
})).With().Timestamp().Logger()
var cli *messagix.Client

func main() {
	cookies := exerrors.Must(cookies.NewCookiesFromFile("session.json", types.Instagram))

	cli = exerrors.Must(messagix.NewClient(types.Instagram, cookies, log, ""))
	cli.SetEventHandler(evHandler)

	err := cli.Connect()
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to connect")
	}

	err = cli.SaveSession("session.json")
	if err != nil {
		log.Fatal().Err(err).Msg("Failed to save session")
	}

	defer func() {
		log.Debug().Msg("Saving session")
		err = cli.SaveSession("session.json")
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to save session")
		}
	}()

	go func() {
		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		<-c
		log.Debug().Msg("Saving session")
		err = cli.SaveSession("session.json")
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to save session")
		}
		os.Exit(0)
	}()

	scan := bufio.NewScanner(os.Stdin)
	for scan.Scan() {
		args := strings.Fields(strings.TrimSpace(scan.Text()))
		var cmd string
		if len(args) > 0 {
			cmd = strings.ToLower(args[0])
		}
		switch cmd {
		case "send":
			tbl, err := cli.Threads.NewMessageBuilder(exerrors.Must(strconv.ParseInt(args[1], 10, 64))).
				SetText(strings.Join(args[2:], " ")).
				Execute()
			if err != nil {
				log.Err(err).Msg("Failed to send message")
			} else {
				log.Info().Any("data", tbl).Msg("Send message response")
			}
		case "quit", "stop", "exit", "q":
			return
		case "save":
			// proceed to save session
		case "":
			continue
		default:
			log.Info().Msg("Unknown command")
		}
		err = cli.SaveSession("session.json")
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to save session")
		}
	}
}

func evHandler(evt any) {
	switch evtData := evt.(type) {
	case *messagix.Event_Ready:
		log.Info().
			Any("connectionCode", evtData.ConnectionCode.ToString()).
			Any("isNewSession", evtData.IsNewSession).
			Msg("Client is ready!")
		_ = json.NewEncoder(exerrors.Must(os.OpenFile("table.json", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600))).Encode(evtData.Table)

		contacts, err := cli.Account.GetContacts(100)
		if err != nil {
			log.Fatal().Err(err).Msg("Failed to get contacts")
		}

		log.Info().Any("data", contacts).Msg("Contacts Response")
	case *messagix.Event_PublishResponse:
		log.Info().Any("evtData", evtData).Msg("Got new event!")
		if len(evtData.Table.LSInsertMessage) > 0 {
			log.Info().Any("data", evtData.Table.LSInsertMessage).Msg("Got new message!")
		}
		if len(evtData.Table.LSInsertXmaAttachment) > 0 {
			fmt.Println(evtData.Table.LSInsertXmaAttachment[0].TargetId)
		}
		if len(evtData.Table.LSInsertAttachmentCta) > 0 {
			fmt.Println(evtData.Table.LSInsertAttachmentCta[0].TargetId)
		}
	case *messagix.Event_Error:
		log.Err(evtData.Err).Msg("The library encountered an error")
		//os.Exit(1)
	case *messagix.Event_SocketClosed:
		log.Info().Any("code", evtData.Code).Any("text", evtData.Text).Msg("Socket was closed, reconnecting")

		go func() {
			err := cli.Connect()
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to connect")
			}

			err = cli.SaveSession("session.json")
			if err != nil {
				log.Fatal().Err(err).Msg("Failed to save session")
			}
		}()
	default:
		log.Info().Any("data", evtData).Interface("type", evt).Msg("Got unknown event!")
	}
}
