package obsws

import (
	"crypto/rand"
	"net/url"
)

const vdoSite = "https://vdo.ninja/"

// VDOPush is the link a phone opens to send its back camera to stream id,
// with no sound, at up to 1080p. Its parameters, from docs.vdo.ninja:
//   - push: https://docs.vdo.ninja/advanced-settings/setup-parameters/push.md
//   - webcam: https://docs.vdo.ninja/advanced-settings/setup-parameters/and-webcam.md
//   - autostart: https://docs.vdo.ninja/advanced-settings/setup-parameters/and-autostart.md
//   - facing: https://docs.vdo.ninja/advanced-settings/mobile-parameters/and-facing.md
//   - quality: https://docs.vdo.ninja/advanced-settings/video-parameters/and-quality.md
//   - audiodevice: https://docs.vdo.ninja/advanced-settings/setup-parameters/audiodevice.md
func VDOPush(id string) string {
	return vdoSite + "?push=" + url.QueryEscape(id) + "&webcam&autostart&facing=rear&quality=0&audiodevice=0"
}

// VDOView is the link the OBS browser source opens to show that phone.
// Its parameters, from docs.vdo.ninja:
//   - view: https://docs.vdo.ninja/getting-started/vdo.ninja-basics.md
//   - noaudio: https://docs.vdo.ninja/advanced-settings/audio-parameters/noaudio.md
//   - cleanoutput: https://docs.vdo.ninja/advanced-settings/design-parameters/cleanoutput.md
//   - videobitrate: https://docs.vdo.ninja/advanced-settings/video-bitrate-parameters/bitrate.md
func VDOView(id string) string {
	return vdoSite + "?view=" + url.QueryEscape(id) + "&noaudio&cleanoutput&videobitrate=4000"
}

// NewVDOID is a random stream id: 20 characters from a-z and 0-9.
func NewVDOID() (string, error) {
	const chars = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 20)
	for i := range b {
		// 252 is the largest multiple of 36 below 256: no letter comes up more.
		var c [1]byte
		for {
			if _, err := rand.Read(c[:]); err != nil {
				return "", err
			}
			if c[0] < 252 {
				break
			}
		}
		b[i] = chars[c[0]%36]
	}
	return string(b), nil
}
