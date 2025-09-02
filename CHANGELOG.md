# v0.5.3 (2025-08-16)

* Deprecated legacy provisioning API. The `/_matrix/provision/v1` endpoints will
  be deleted in the next release.
* Bumped minimum Go version to 1.24.
* Added automatic refresh if sending media fails with please reload page error.
* Added time limit for using cached connection states.

# v0.5.2 (2025-07-16)

* Fixed handling some types of GraphQL errors.
* Fixed LSVersion finding error on some accounts.

# v0.5.1 (2025-06-16)

* Fixed deadlock on websocket disconnect (introduced in v0.5.0).

# v0.5.0 (2025-06-16)

* Added option to reconnect faster after restart by caching connection state.
* Added option to allow both Messenger and Facebook login without allowing
  Instagram.
* Added basic support for direct media.
* Added placeholders for encrypted messages on Instagram.
* Updated Docker image to Alpine 3.22.
* Changed Instagram reel bridging to never include caption.
* Fixed detecting some types of logouts.

# v0.4.6 (2025-04-16)

* Fixed bridging own read status in encrypted chats.
* Fixed proxy not being used during login.

# v0.4.5 (2025-03-16)

* Added experimental fix for fetching missing names in encrypted chats.
* Added fallback for unsupported encrypted messages.

# v0.4.4 (2025-02-16)

* Bumped minimum Go version to 1.23.
* Added support for signaling supported features to clients using the
  `com.beeper.room_features` state event.
* Added auto-reconnect for certain uncommon error cases.
* Fixed replies with images including an extra blank message.

# v0.4.3 (2024-12-16)

* Fixed PNGs with certain color models failing to render on native apps
  (by automatically converting all PNGs to NRGBA).
* Fixed failed message sends not triggering an error message in certain cases.
* Updated Docker image to Alpine 3.21.

# v0.4.2 (2024-11-16)

* No notable changes

# v0.4.1 (2024-10-16)

* Fixed bridging newlines in messages from Meta that include user mentions.
* Added option to apply proxy settings to media downloads.

# v0.4.0 (2024-09-16)

* Bumped minimum Go version to 1.22.
* Rewrote bridge using bridgev2 architecture.
  * It is recommended to check the config file after upgrading. If you have
    prevented the bridge from writing to the config, you should update it
    manually.

# v0.3.2 (2024-07-16)

* Fixed own ghost user's avatar being reset on bridge restart.
* Added automatic reconnect on WhatsApp 415 errors.
* Improved fallback messages for some message types.

# v0.3.1 (2024-05-16)

* Added option to disable fetching XMA media (reels and such) entirely.
  * The URL to the reel will be included in the caption when fetching is
    disabled.
* Added periodic refresh to avoid refresh errors when sending media.
* Fixed many different cases of media uploads failing.
* Fixed sending replies in encrypted chats.

# v0.3.0 (2024-04-16)

* Added mautrix-facebook database migration utility
  (see [docs](https://docs.mau.fi/bridges/go/meta/facebook-migration.html) for more info).
* Added support for automatically detecting when Messenger force-enables
  encryption in a chat (rather than having to wait for the first incoming
  message).
* Added bridging of chat deletes from Meta to Matrix.
* Changed reel handling to include URL in caption in addition to the
  `external_url` field if downloading the full video fails.
* Started ignoring weird sync timeouts from Messenger that don't break anything.
* Fixed uploading media when Meta servers return the media ID as a string.
* Fixed messages being double encrypted in legacy backfill.
* Fixed backfilled messages not having appropriate `fi.mau.double_puppet_source` key.
* Fixed searching for Messenger users.

# v0.2.0 (2024-03-16)

* Added retries if HTTP requests to Meta fail.
* Added link preview bridging from Meta in [MSC4095] format.
* Added proper handling for some errors in CAT refreshing
  and media uploads to Meta.
* Made initial connection less hacky.
* Reduced number of required cookies.
* Fixed handling some types of Instagram reel shares.
* Fixed WhatsApp voice message bridging.
* Fixed handling edits from WhatsApp.

[MSC4095]: https://github.com/matrix-org/matrix-spec-proposals/pull/4095

# v0.1.0 (2024-02-16)

Initial release.
