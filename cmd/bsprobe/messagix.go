package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"regexp"
	"time"

	"github.com/rs/zerolog"

	"go.mau.fi/mautrix-meta/pkg/messagix"
	"go.mau.fi/mautrix-meta/pkg/messagix/cookies"
	messagixTable "go.mau.fi/mautrix-meta/pkg/messagix/table"
	"go.mau.fi/mautrix-meta/pkg/messagix/types"
)

// cmdMessagix is an acceptance probe for the production decoder. Unlike the
// HAR classifier, it loads a real Business Suite Page through pkg/messagix.
// It intentionally prints only counts and IDs, never cookies or message data.
func cmdMessagix(args []string) error {
	fs := flag.NewFlagSet("messagix", flag.ExitOnError)
	sessionPath := fs.String("session", defaultSessionFile, "session file")
	assetID := fs.String("asset", "", "selected Page asset ID")
	actorID := fs.String("actor", "", "profile-switcher actor ID to resolve")
	username := fs.String("username", "", "public Page username used by the resolver")
	connect := fs.Bool("connect", false, "also require a successful production socket sync")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *assetID == "" && *actorID == "" {
		return fmt.Errorf("--asset or --actor is required")
	}

	session, err := LoadSession(*sessionPath)
	if err != nil {
		return err
	}
	values := make(map[cookies.MetaCookieName]string, len(session.Cookies))
	for _, cookie := range session.Cookies {
		values[cookies.MetaCookieName(cookie.Name)] = cookie.Value
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if *actorID != "" {
		fbCookies := &cookies.Cookies{Platform: types.Facebook}
		fbCookies.UpdateValues(values)
		fbClient := messagix.NewClient(fbCookies, zerolog.New(io.Discard), &messagix.Config{})
		profile := types.SwitchableProfile{ID: *actorID, Username: *username, Name: "Page"}
		if err := fbClient.DiscoverSwitchableProfiles(ctx); err == nil {
			for _, candidate := range fbClient.GetSwitchableProfiles() {
				if candidate.ID == *actorID {
					profile = candidate
					break
				}
			}
		}
		candidates := regexp.MustCompile(`\d{6,}`).FindAllString(profile.AvatarURL, -1)
		fmt.Printf("profile metadata: username_present=%t avatar_id_candidates=%v\n", profile.Username != "", candidates)
		resolved, err := fbClient.ResolveBusinessSuitePage(ctx, profile)
		if err != nil {
			return err
		}
		*assetID = resolved.AssetID
	}
	c := &cookies.Cookies{Platform: types.BusinessSuite}
	c.UpdateValues(values)
	client := messagix.NewClient(c, zerolog.New(io.Discard), &messagix.Config{
		BusinessSuite: &messagix.BusinessSuiteContext{AssetID: *assetID, PageID: *assetID},
	})

	user, table, err := client.LoadMessagesPage(ctx)
	if err != nil {
		return err
	}
	fmt.Printf("production parser accepted Business Suite: user_id=%d page_asset=%s table_present=%t\n", user.GetFBID(), *assetID, table != nil)
	if table != nil {
		threadGroups := make(map[int64]int)
		for _, thread := range table.LSDeleteThenInsertThread {
			threadGroups[thread.SyncGroup]++
		}
		fmt.Printf(
			"Business Inbox snapshot decoded: fields=%d threads=%d messages=%d participants=%d sync_transactions=%d thread_groups=%v ig_thread_markers=%d\n",
			len(table.NonNilFields()),
			len(table.LSDeleteThenInsertThread),
			len(table.LSUpsertMessage),
			len(table.LSAddParticipantIdToGroupThread),
			len(table.LSExecuteFirstBlockForSyncTransaction),
			threadGroups,
			len(table.LSDeleteThenInsertIgThreadInfo),
		)
	}
	if *connect {
		events := make(chan any, 8)
		client.SetEventHandler(func(_ context.Context, event any) { events <- event })
		if err := client.Connect(ctx); err != nil {
			return err
		}
		defer client.Disconnect()
		for {
			select {
			case event := <-events:
				switch typed := event.(type) {
				case *messagixTable.LSTable:
					threadGroups := make(map[int64]int)
					for _, thread := range typed.LSDeleteThenInsertThread {
						threadGroups[thread.SyncGroup]++
					}
					fmt.Printf(
						"socket table decoded: fields=%v threads=%d updated_threads=%d verified_threads=%d messages=%d inserted_messages=%d thread_groups=%v ig_thread_markers=%d\n",
						typed.NonNilFields(),
						len(typed.LSDeleteThenInsertThread), len(typed.LSUpdateOrInsertThread),
						len(typed.LSVerifyThreadExists), len(typed.LSUpsertMessage),
						len(typed.LSInsertMessage), threadGroups,
						len(typed.LSDeleteThenInsertIgThreadInfo),
					)
				case *messagix.ConnectedEvent, *messagix.ReconnectedEvent:
					fmt.Println("production socket sync accepted Business Suite")
					return nil
				case *messagix.PermanentErrorEvent:
					return typed.Err
				case *messagix.TransientDisconnectEvent:
					return typed.Err
				}
			case <-ctx.Done():
				return fmt.Errorf("timed out waiting for production socket sync: %w", ctx.Err())
			}
		}
	}
	return nil
}
