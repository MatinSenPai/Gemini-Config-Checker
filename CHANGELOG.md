# Changelog

All notable changes are documented here. The format follows [Keep a Changelog](https://keepachangelog.com/) and the project uses [Semantic Versioning](https://semver.org/).

## [Unreleased]

## [0.1.0]

### Fixed
- The browser is detected again on every check (a browser installed while the app was open is found), in more places on every OS (Windows Program Files / per-user installs, macOS ~/Applications, Linux snap and absolute paths when a launcher starts the app with a short PATH, Vivaldi, Chromium), and `GCC_BROWSER` can point at one. Android now says plainly that apps cannot drive an installed browser, instead of reporting it as missing.

- Where no controllable browser exists (Android, headless servers) the check type is locked to connectivity-only, a notice says so before the scan starts, and the button reads "start connectivity test" instead of silently switching modes.

### Added
- **Simple and Advanced modes** with a prominent animated switch. Simple tests each of your own configs directly, with no chain (anti-DPI profile applied to every config, links / subscription URL / base64 accepted, full-config export per config). Advanced is the chained scan below.
- Chained scan: every free config is tested behind the user's own base config.
- Region check in a signed-in browser (AI Studio and Gemini reported separately; configurable pass criteria).
- Local Google sign-in through the user's Chrome / Edge / Brave in an isolated profile; session cookies stay in memory.
- Anti-DPI profile (finalmask fragments, fingerprint, ALPN, cipher suites, clean IP) with an in-process fragmenting forwarder.
- Anti-DPI values are read from a pasted config under every common spelling: cipher suites as a string or an array (`cipherSuites` / `ciphers` / `cipher_suites`), ALPN as a string or an array, link parameters in any letter case, and the same values in vmess links.
- Base config as a share link or a full xray JSON (multi-hop chains supported); anti-DPI values can be read from a pasted config.
- Copy with chain: standalone xray config in the layout v2rayN / PattN export, for PattN (with finalmask) or stock xray.
- A sign-in redirect must persist for several seconds before the app treats the Google session as expired, so a transient redirect while a page loads no longer aborts a scan.
- Connectivity-only mode for machines without a browser (Android / Termux, servers).
- New logo (chain "A" with fangs and a Gemini spark) on every platform: desktop, web, README, and a properly centred Android adaptive icon.
- Desktop app (Wails) and web server sharing one engine and UI; Persian UI with light and dark themes.
- Android APK (arm64-v8a): the engine runs as a foreground service behind a WebView; connectivity mode only.
- CI (gofmt, vet, tests on three OSes) and a release pipeline that runs on push to `main` (Windows, macOS, Linux desktop; web builds for Linux, Windows, macOS; Android APK) and publishes `v<VERSION>` with checksums.
