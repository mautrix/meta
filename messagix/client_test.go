package messagix_test

import (
	"log"
	"os"
	"testing"
	"github.com/0xzer/messagix"
	"github.com/0xzer/messagix/cookies"
	"github.com/0xzer/messagix/debug"
	"github.com/0xzer/messagix/table"
	"github.com/0xzer/messagix/types"
)

var cli *messagix.Client
func TestClient(t *testing.T) {
	session := &cookies.InstagramCookies{}
	err := cookies.NewCookiesFromString(``, session)
	if err != nil {
		log.Fatalf("failed to create insta cookies: %e", err)
	}

	cli, err = messagix.NewClient(types.Instagram, session, debug.NewLogger(), "")
	if err != nil {
		log.Fatal(err)
	}
	cli.SetEventHandler(evHandler)

	err = cli.Connect()
	if err != nil {
		log.Fatalf("failed to connect to socket: %e", err)
	}

	cli.SaveSession("test_files/session.json")
	// making sure the main program does not exit so that the socket can continue reading
	wait := make(chan struct{})
    <-wait
}

func evHandler(evt interface{}) {
	switch evtData := evt.(type) {
		case *messagix.Event_Ready:
			cli.Logger.Info().
			//Any("threads", evtData.Table.LSDeleteThenInsertThread).
			// Any("setcontentdisplay", evtData.Table.LSSetMessageDisplayedContentTypes).
			Msg("Client is ready!")
			threads := evtData.Table.LSDeleteThenInsertThread
			for _, thread := range threads {
				cli.Logger.Info().Any("thread_name", thread.ThreadName).Any("thread_key", thread.ThreadKey).Msg("[READY] Got thread info!")
			}
		case *messagix.Event_PublishResponse:
			// cli.Logger.Info().Any("tableData", evtData.Table).Msg("Received new event from socket")
			threads := evtData.Table.LSDeleteThenInsertThread
			for _, thread := range threads {
				cli.Logger.Info().Any("thread_name", thread.ThreadName).Any("thread_key", thread.ThreadKey).Msg("[PUBLISH] Got thread info!")
			}
		case *messagix.Event_Error:
			cli.Logger.Err(evtData.Err).Msg("The library encountered an error")
			os.Exit(1)
		case *messagix.Event_SocketClosed:
			cli.Logger.Info().Any("code", evtData.Code).Any("text", evtData.Text).Msg("Socket was closed.")
			os.Exit(1)
		default:
			cli.Logger.Info().Any("data", evtData).Interface("type", evt).Msg("Got unknown event!")
	}
}

func fetchMessages() {
	currentCursor := cli.SyncManager.GetCursor(1)
	resp, err := cli.Threads.FetchMessages(61550046156682, 1694956983720, "mid.$cAABtBT1ku_GQ1JRdqGKo08VYGlmT", currentCursor)
	if err != nil {
		log.Fatalf("failed to fetch messages: %e", err)
	}
	cli.Logger.Debug().Any("resp", resp).Msg("fetch messages")
}

func sendReaction() {
	resp, err := cli.Messages.SendReaction(11111, "mid", "👇")
	if err != nil {
		log.Fatalf("failed to send reaction: %e", err)
	}
	log.Println(resp.LSReplaceOptimisticReaction)
}

func sendMessageWithMedia() {
	mediaData, _ := os.ReadFile("test_files/test.jpeg")
	turtleData, _ := os.ReadFile("test_files/turtle.jpeg")

	medias := []*messagix.MercuryUploadMedia{
		{Filename: "test_image.jpg", MediaType: messagix.IMAGE_JPEG, MediaData: mediaData},
		{Filename: "turtle.jpg", MediaType: messagix.IMAGE_JPEG, MediaData: turtleData},
	}

	mediaUploads, err := cli.SendMercuryUploadRequest(medias)
	if err != nil {
		log.Fatalf("failed: %e", err)
	}

	cli.Logger.Info().Any("uploads", mediaUploads).Msg("Media uploads")

	sendMsgBuilder := cli.Threads.NewMessageBuilder(1111111)
	sendMsgBuilder.SetMedias(mediaUploads)
	sendMsgBuilder.SetText("media test :D")
	sendMsgBuilder.SetLastReadWatermarkTs(1696261558117)

	tableResp, err := sendMsgBuilder.Execute() // make sure to execute to send the task
	if err != nil {
		log.Fatalf("failed to send media: %e", err)
	}

	log.Println(tableResp)
}

func sendMessageText() {
	msgBuilder := cli.Threads.NewMessageBuilder(11111)
	msgBuilder.SetInitiatingSource(table.FACEBOOK_INBOX)
	msgBuilder.SetLastReadWatermarkTs(16962611558117)
	msgBuilder.SetSource(table.MESSENGER_INBOX_IN_THREAD)
	msgBuilder.SetText("hello there")
	tableResp, err := msgBuilder.Execute()
	if err != nil {
		log.Fatalf("failed to send text msg: %e", err)
	}
	log.Println(tableResp)
}