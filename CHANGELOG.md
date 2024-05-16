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
