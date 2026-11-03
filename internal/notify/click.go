package notify

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Bundle IDs and app names of the source apps.
const (
	WhatsAppBundle = "net.whatsapp.WhatsApp"
	appMattermost  = "Mattermost"
	appWhatsApp    = "WhatsApp"
)

// Apps reports whether an app (for example "Mattermost") is installed.
type Apps func(name string) bool

// InstalledApp reports whether <name>.app is in /Applications or ~/Applications.
func InstalledApp(name string) bool {
	dirs := []string{"/Applications"}
	if home, err := os.UserHomeDir(); err == nil {
		dirs = append(dirs, filepath.Join(home, "Applications"))
	}
	for _, d := range dirs {
		if fi, err := os.Stat(filepath.Join(d, name+".app")); err == nil && fi.IsDir() {
			return true
		}
	}
	return false
}

// AllowedOpen reports whether a click can open u: only https, mattermost and
// whatsapp URLs.
func AllowedOpen(u string) bool {
	p, err := url.Parse(u)
	if err != nil || p.Host == "" && p.Scheme != "whatsapp" {
		return false
	}
	switch p.Scheme {
	case "https", "mattermost", "whatsapp":
		return true
	}
	return false
}

// Click returns the click target of a stored message URL of the source type
// ("mattermost" or "whatsapp"): the URL to open, or the bundle ID of the app
// to bring to the front. The URL comes from the store, never from card text.
func Click(sourceType, msgURL string, apps Apps) (open, activate string) {
	if apps == nil {
		apps = InstalledApp
	}
	switch sourceType {
	case "mattermost":
		if !AllowedOpen(msgURL) || !strings.HasPrefix(msgURL, "https://") {
			return "", ""
		}
		if strings.Contains(msgURL, "/pl/") && apps(appMattermost) {
			return "mattermost://" + strings.TrimPrefix(msgURL, "https://"), ""
		}
		return msgURL, ""
	case "whatsapp":
		if !apps(appWhatsApp) {
			if AllowedOpen(msgURL) && strings.HasPrefix(msgURL, "https://") {
				return msgURL, ""
			}
			return "", ""
		}
		if phone, ok := strings.CutPrefix(msgURL, "https://wa.me/"); ok && phone != "" && strings.Trim(phone, "0123456789") == "" {
			return "whatsapp://send?phone=" + phone, ""
		}
		return "", WhatsAppBundle
	}
	if AllowedOpen(msgURL) && strings.HasPrefix(msgURL, "https://") {
		return msgURL, ""
	}
	return "", ""
}
