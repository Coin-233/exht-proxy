package proxy

import _ "embed"

var (
	//go:embed assets/mobile.html
	mobileAppHTML string

	//go:embed assets/gallery.html
	mobileViewHTML string

	//go:embed assets/viewer.html
	mobileViewerHTML string

	//go:embed assets/stats.html
	injectedUI string

	//go:embed assets/translations.txt
	defaultTranslations string
)
