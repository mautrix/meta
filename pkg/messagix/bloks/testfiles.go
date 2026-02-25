package bloks

import (
	_ "embed"
)

//go:embed debug_captcha.png
var debugImageCaptcha []byte

//go:embed debug_captcha.ogg
var debugAudioCaptcha []byte
