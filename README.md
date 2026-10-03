<p align="center"><img src="assets/banner.png" alt="Gemini/Check" width="100%"></p>

<p align="center">
  <b>English</b> · <a href="README.fa.md">فارسی</a>
</p>

<p align="center">
  <a href="../../releases"><img alt="Release" src="https://img.shields.io/github/v/release/MatinSenPai/Gemini-Config-Checker?include_prereleases&label=release"></a>
  <a href="../../releases"><img alt="Downloads" src="https://img.shields.io/github/downloads/MatinSenPai/Gemini-Config-Checker/total?label=downloads&color=ee2b38"></a>
  <a href="../../actions/workflows/ci.yml"><img alt="CI" src="https://github.com/MatinSenPai/Gemini-Config-Checker/actions/workflows/ci.yml/badge.svg"></a>
  <img alt="Platforms" src="https://img.shields.io/badge/platforms-Windows%20%7C%20macOS%20%7C%20Linux%20%7C%20Android-111">
</p>

# Gemini Config Checker

Find proxy configs that actually get you into **Google AI Studio and Gemini**, by testing each one the way a person would: in a
*signed-in* browser, through the route you will really use.

> **Version 0.1.0, early pre-release.** It works end to end on Windows (the only platform it has been run on so far). The macOS,
> Linux and Android builds come from the CI pipeline and have not been exercised on real devices yet. Please report what you find.

## Two modes, one switch

The big **Simple / Advanced** switch in the header decides what is tested. The route diagram under the title morphs to show it.

| | **Simple** | **Advanced** |
|---|---|---|
| Question it answers | *Which of my configs work?* | *Which free configs work through my config?* |
| Route | you → **config** → AI Studio / Gemini | you → **base config** → **free config** → AI Studio / Gemini |
| You provide | your configs (links, a subscription URL or base64), or pick a ready-made free list | one working base config, plus a free list or pasted configs |
| Anti-DPI profile is applied to | every config under test (each one connects from your machine) | the base config's first hop (free configs run inside its tunnel) |
| Typical use | check a pile of configs you already have; test a config before you rely on it | your Cloudflare config works, but you need a *different exit* that Google accepts |
| Export | links, base64, a full Xray config of one config | links, base64, a chain Xray config (`base → free`) |

## Contents
[Credits](#credits-and-references) · [Why a signed-in browser](#why-a-signed-in-browser) · [Download and install](#download-and-install) ·
[User guide](#user-guide) · [Troubleshooting](#troubleshooting-and-faq) · [Privacy and security](#privacy-and-security) ·
[Build from source](#build-from-source) · [Releases and CI](#releases-and-ci)

## Credits and references

This project exists because of other people's work. Please go and use and star them.

| Project | What it gave us |
|---|---|
| [**0xRadikal/Free-v2ray-Configs**](https://github.com/0xRadikal/Free-v2ray-Configs) | The auto-updated lists of free configs (`verified`, `fast`, `secure`, ...) that this tool scans |
| [**patterniha**](https://github.com/patterniha) | [PattN](https://github.com/patterniha/PattN) (Windows / Linux / macOS client) and [PattNG](https://github.com/patterniha/PattNG) (Android): the anti-DPI recipe, plus the fragment `finalmask` semantics from his [Xray-core fork](https://github.com/patterniha/Xray-core) that our fragmenting forwarder reproduces |
| [**bia-pain-bache/BPB-Worker-Panel**](https://github.com/bia-pain-bache/BPB-Worker-Panel) | The Cloudflare Worker panel that produces many of the base configs this tool chains behind (patterniha maintains a [fork](https://github.com/patterniha/BPB-Worker-Panel)) |
| [**Anti-DPI recipe post**](https://t.me/MatinSenPaii/5469) | The exact Finalmask / fingerprint / ALPN / cipher values used as defaults |
| [**MatinSenPai/SenPaiScanner**](https://github.com/MatinSenPai/SenPaiScanner) | Companion tool for finding clean Cloudflare IPs to use as a config's address |
| [XTLS/Xray-core](https://github.com/XTLS/Xray-core) | The proxy engine embedded in the app |
| [2dust/v2rayN](https://github.com/2dust/v2rayN) | The config layout the "Copy with chain" export follows (PattN is a fork of it) |
| [Wails](https://wails.io) | The native desktop shell |
| [Vazirmatn](https://github.com/rastikerdar/vazirmatn) and [JetBrains Mono](https://github.com/JetBrains/JetBrainsMono) | The fonts used in the UI (SIL Open Font License) |

## Why a signed-in browser?

Google decides region availability **per signed-in account, after the page has loaded**. A plain HTTP request (even to the Gemini API
with a dummy key) answers "fine" from every country, so connectivity probes mark every live config as healthy. Measured while building
this tool: exits geolocated to the US, HK and SG were blocked, and the same exit can flip over time. The only reliable signal is what
Google shows a signed-in user, so that is what this tool reads:

| Product | "Blocked" looks like |
|---|---|
| AI Studio | redirect to `/docs/available-regions` |
| Gemini | "Gemini isn't currently supported in your country" |

## Download and install

Get the build for your system from the [Releases](../../releases) page. Every release ships `SHA256SUMS.txt`.

| Platform | Desktop app (native window) | Web server (UI in your browser) | Region check |
|---|---|---|---|
| Windows | amd64, arm64 | amd64, arm64 | ✅ needs Chrome / Edge / Brave |
| macOS | universal (Intel + Apple Silicon) | amd64, arm64 | ✅ needs Chrome / Edge / Brave |
| Linux | amd64 (GTK 3 + WebKitGTK 4.1) | amd64, arm64, armv7 | ✅ needs Chrome / Chromium / Edge / Brave |
| Android 8+ | **APK** (arm64-v8a) | build from source for Termux (`GOOS=android`) | ❌ connectivity mode only |

<details><summary><b>Windows</b></summary>

1. Download `gemini-config-checker-desktop_<version>_windows_amd64.zip` (or `arm64`) and unzip it anywhere.
2. Run `GeminiConfigChecker.exe`. WebView2 is built into Windows 10/11; if SmartScreen warns, choose *More info → Run anyway* (the build is unsigned).
</details>

<details><summary><b>macOS</b></summary>

1. Download `..._macos_universal.zip`, unzip, and move the `.app` to *Applications*.
2. First launch: right-click the app → *Open*. If macOS still refuses: `xattr -dr com.apple.quarantine "/Applications/GeminiConfigChecker.app"`.
</details>

<details><summary><b>Linux</b></summary>

1. Install the runtime libraries once: `sudo apt install libgtk-3-0 libwebkit2gtk-4.1-0` (Debian/Ubuntu; use your distribution's equivalents).
2. `tar -xzf gemini-config-checker-desktop_<version>_linux_amd64.tar.gz`, then run `./GeminiConfigChecker`.
3. No desktop environment? Use the **web server** build instead: `./gemini-config-checker` and open `http://localhost:8787`.
</details>

<details><summary><b>Android</b></summary>

1. Download `gemini-config-checker_<version>_android_arm64.apk` on the phone.
2. Allow *Install unknown apps* for your browser or file manager, then open the APK.
3. Launch the app. A persistent notification ("Scanner engine is running") is expected: the engine runs as a foreground service so long scans survive the screen turning off. Use its **Stop** action to quit the engine.
4. The APK runs the **connectivity-only** mode (see [Settings reference](#7-settings-reference)). Android does not let apps drive a signed-in Chromium, so the region cannot be verified there.
5. The release APK is signed with the repository key when one is configured, otherwise with a throw-away key: it installs, but a later build cannot update it in place, so uninstall first.
</details>

<details><summary><b>Web server (any OS, servers, Termux)</b></summary>

`./gemini-config-checker` serves the same UI on `http://localhost:8787` and opens your browser. Flags: `-addr 127.0.0.1:8787`, `-no-open`, `-version`.
The UI has **no authentication**: keep it on localhost, or tunnel it (`ssh -L 8787:localhost:8787 your-server`) instead of exposing it.
</details>

## User guide

### 1. First start
The window has three tabs: **Scan** (what to test and how), **Results** (live list, filters, copy buttons) and **Export**.
The header holds the **Simple / Advanced** switch, the scan status, and a light/dark theme button. Everything you type is remembered on this
device when "remember" is ticked (untick it on a shared computer).

### 2. Pick a mode
* Use **Simple** when you already have configs and want to know which ones work, or to verify the config you are about to rely on.
* Use **Advanced** when your Cloudflare config connects fine but Google blocks its exit: free configs chained behind it give you other exits.
* You can flip the switch at any time. Results keep the mode of the scan that produced them; Export and the copy buttons follow that.

### 3. Sign in with Google (region check)
The region check opens AI Studio and Gemini as *you*. Click **Sign in with Google** once:
1. A Chrome / Edge / Brave window opens in a **separate profile** that only this app uses. Sign in normally. The app never sees your password.
2. When sign-in is detected the window closes by itself and the card turns to "Google account connected". The sign-in is remembered for next time.
3. Prefer a **secondary account**: requests come from many IPs, so Google may show a "suspicious sign-in" notice.
4. **Remove account from app** deletes only that private profile. It never touches your everyday browser or accounts.

No browser installed, or on Android? Switch the *check type* (Settings) to **connectivity only**: no sign-in or browser needed.

### 4. Simple mode, step by step
1. Paste your configs into the box, one per line: `vless://`, `vmess://`, `trojan://`, `ss://`. A subscription URL (`https://…`) or base64 text also works.
   Leave the box empty to test a ready-made free list instead (pick one below).
2. Keep **Anti-DPI** on if you are inside a network where plain Cloudflare configs do not connect (see [section 6](#6-the-anti-dpi-profile)).
3. Sign in (region check) or choose connectivity only.
4. Press **Start scan** (or `Ctrl`+`Enter`). Watch the **Results** tab fill in.

### 5. Advanced mode, step by step
1. **Base config**: a config that works for you and does *not* get the region wall on AI Studio. Paste a share link, or a full xray JSON, even a multi-hop chain
   (for example an exit that already dials through a Cloudflare worker). This is the route every free config will be chained behind.
2. **Anti-DPI**: applied to the base config's first hop, the connection that leaves your machine.
3. **Source**: choose a ready-made list (`verified` is recommended) or paste free configs yourself. `custom` accepts any subscription URL.
4. Settings, sign-in, **Start scan**. Before testing the free configs, the app tests your base config first and stops with a clear message if the base itself cannot reach Google.

### 6. The anti-DPI profile
Inside Iran, plain Cloudflare configs mostly connect only with the settings below
([recipe by patterniha / PattN](https://t.me/MatinSenPaii/5469)). The values can change, so every one is editable; **Suggested values** restores the defaults.

| Setting | Default | What it does |
|---|---|---|
| Finalmask (JSON) | two `fragment` masks: `tlshello` with `lengths [0,104,1]`, then `1-1` with `lengths [114,1]` | splits the TLS ClientHello so filters cannot read the server name |
| Fingerprint | `unsafe` | native Go TLS instead of a browser imitation, so the cipher list below applies |
| ALPN | `http/1.1` | |
| Cipher suites | the 13-suite list from the recipe | |
| Clean IP | empty | replaces the connection address (the old domain stays as SNI and Host) |

* **Read from config**: if the config you pasted already carries these values (`finalmask`, `cipherSuites`, `alpn` in JSON, or `fm`, `ciphers`, `alpn`, `fp` link parameters), this button copies them into the form.
* Stock xray-core only supports one `length`/`delay` per fragment mask, while the recipe uses per-segment `lengths`/`delays` (a PattN fork feature). The app runs the same
  fragmenting algorithm in a small local forwarder in front of the connection. Only `fragment` masks are supported; other mask types are rejected with a clear message.
* Switch the profile off outside filtered networks; it only adds latency there.

### 7. Settings reference
| Setting | Default | Meaning |
|---|---|---|
| Check type | Region with signed-in browser | **Region**: opens AI Studio and Gemini and reads Google's verdict (accurate, 20–40 s per config). **Connectivity only**: just proves the route reaches Google and measures latency (fast, no sign-in, but the region is *not* checked) |
| "Healthy" means | AI Studio **and** Gemini | Require both, only AI Studio, or only Gemini |
| Max configs | 60 | How many configs to test (the list is shuffled first) |
| Simultaneous browser checks | 4 | Each uses about 200 MB of RAM; lower it on small machines |
| Simultaneous connectivity tests | 20 | The cheap first stage that drops dead configs before the browser stage |
| Connectivity timeout | 10 s | Raise it on slow links |
| Remember | on | Stores configs and settings in the app's local storage on this device only |

### 8. Reading the results
* **Healthy / Connected**: white label. In region mode it means your "healthy" rule passed; in connectivity mode it only means "reachable".
* **Region blocked**: Google showed the region wall for at least one product you require. The **AI Studio** and **Gemini** columns show each product (`✓ free`, `✗ blocked`).
* **Error**: dead config, timeout, or a browser problem; hover the label for the reason.
* **Ping** is the latency of one request through the full route. The first tile shows the number tested, and "base ping" (advanced) or "lowest ping" (simple).
* Filter by status, search by name or address, sort by ping or name, and select several rows for bulk copy.
* **A verdict is a sample, not a guarantee.** The exit of a Cloudflare Worker can change between connections, so the same route may pass now and fail a minute later. Re-test the config you intend to keep.

### 9. Using what you found
* **Copy** puts the share link on the clipboard. Paste it into your client (v2rayN, Hiddify, PattN, ...), or into BPB's **Chain Proxy** field.
* **Copy with chain** (advanced) / **Copy full config** (simple) puts a ready-to-import Xray config on the clipboard. The Export tab does the same and can save a `.json` file. Choose the target client:

| Target | Option | `finalmask` in the file |
|---|---|---|
| PattN / PattNG | *PattN / PattNG (with finalmask)* | included |
| Xray / v2rayN with the stock core | *Xray / v2rayN standard* | removed (stock cores cannot parse the fork's schema) |

  The file uses the classic `vnext` / `servers` layout (old and new cores accept it), mixed inbounds on `10808` and `10809`, pinned DNS and a UDP/443 block.
  In v2rayN: *Servers → Add custom configuration server* and pick the file. To try it without a GUI: `xray run -c file.json`.
* **Export tab**: links (one per line), a base64 subscription, or the Xray config; limit how many, copy, or save.

### 10. Android notes
Same UI, connectivity mode only, default Simple. The foreground notification keeps the engine alive during a scan; tapping it reopens the app, **Stop** shuts the engine down.
"Save to file" opens Android's file picker.

## Troubleshooting and FAQ

| Symptom | What to do |
|---|---|
| *"No Chrome / Edge / Brave found"* | Install one, or choose **connectivity only**. Searched: Windows Program Files / per-user installs / the registry "App Paths" (Chrome, Edge, Brave, Chromium, Vivaldi, Opera); macOS `/Applications`, `~/Applications` and Spotlight; Linux `PATH`, `/usr/bin`, `/snap/bin`, `/opt/...`. A browser somewhere else: set `GCC_BROWSER` (or `CHROME_BIN`) to its executable. Flatpak browsers cannot be driven. |
| Linux: sign-in window does not open | The window needs a graphical session (`DISPLAY` / `WAYLAND_DISPLAY`); on a server, sign in on a desktop. Snap browsers work (the profile is kept under `~/snap/<browser>/common`); as root the browser starts with `--no-sandbox`. |
| *"Google session expired, sign in again"* | Press **Sign in with Google** again. A sign-in redirect must persist for several seconds before the app believes it, so a brief redirect no longer aborts a scan; if it still happens, Google really asked you to sign in again (often a security check after many IPs). |
| Everything times out | The base config (advanced) or the configs themselves (simple) cannot connect. Inside a filtered network turn the **anti-DPI profile** on and try a **clean IP**; make sure the base config works in your normal client. |
| AI Studio passes but Gemini is blocked (or the reverse) | They use different region rules. Change *"Healthy" means* if you only need one. |
| A config passed, then failed later | See "A verdict is a sample": the exit varies per connection. Re-test before relying on it. |
| *"A scan is already running"* | Stop it first (the red button in the bar or in Results). |
| Windows SmartScreen / macOS Gatekeeper warns | The builds are unsigned. Use *Run anyway* / right-click → Open (see [Download](#download-and-install)). |
| Linux: window does not open | Install `libgtk-3-0` and `libwebkit2gtk-4.1-0`, or use the web server build. |
| Android: install blocked | Allow "Install unknown apps" for the app you used to open the APK. |
| Android: *"App not installed as package appears to be invalid"* | The file is damaged or incomplete (a half-finished download, or a repository page saved as `.apk`). Download it again, check its size and `SHA256SUMS.txt`, or install it from a computer with `adb install file.apk`. It also appears when an APK signed with a different key is installed over an older build: uninstall the old app first. |
| Start over completely | **Remove account from app**, then delete the app's local storage (Windows: `%AppData%\GeminiConfigChecker`; macOS: `~/Library/Application Support/GeminiConfigChecker`; Linux: `~/.config/GeminiConfigChecker`). |

## Privacy and security
* Sign-in happens in your browser, not in the app; cookies live in memory only. The isolated profile never touches your everyday browser profile or accounts.
* Traffic to Google is end-to-end TLS; a free-config operator sees destinations, not content. Do not push sensitive traffic through free configs.
* Your configs and settings are stored only in the app's local storage, and only if "remember" is ticked. They are never sent anywhere except through your own routes.
* The web server binds to `127.0.0.1` and rejects foreign `Host` headers and non-JSON POSTs, but it has no authentication.

Details and reporting: [SECURITY.md](SECURITY.md).

## Build from source
Requires Go (see `go.mod`). The desktop app additionally needs [Wails v2](https://wails.io) and the platform webview
(WebView2 on Windows, WebKit on macOS, GTK 3 + WebKitGTK 4.1 on Linux). The Android app needs the Android SDK, JDK 17+ and Gradle 8.10.

```sh
make test                      # vet + unit tests
make web                       # ./dist/gemini-config-checker
make desktop                   # native app via Wails (on the target OS)
make android                   # APK (engine for arm64 + Gradle)
make dist VERSION=0.1.0        # cross-compiled web binaries for all targets
```

## Releases and CI
* `ci.yml`: gofmt, vet and tests on Linux / Windows / macOS; cross-compile check for every web target.
* `release.yml`: builds desktop apps (Windows amd64/arm64, macOS universal, Linux amd64), web servers (Linux, Windows, macOS) and the Android APK, writes `SHA256SUMS.txt`
  and publishes the GitHub release `v<VERSION>`. **It runs by itself only when a push to `main` changes `VERSION`** (ordinary pushes run just `ci.yml`): if that release does not exist yet it is
  built and published, otherwise the run does nothing. To ship a new version, bump `VERSION` and `CHANGELOG.md` and push. A tag `vX.Y.Z` matching `VERSION`, or
  *Actions → Release → Run workflow* (tick "publish", also how you retry a failed release), works too.
* Optional repository secrets keep one Android signature across releases: `ANDROID_KEYSTORE_BASE64`, `ANDROID_KEYSTORE_PASSWORD`, `ANDROID_KEY_ALIAS`, `ANDROID_KEY_PASSWORD`.

## Project layout
```
main.go              web server (HTTP API + embedded UI)
desktop/             Wails desktop shell around the same engine and UI
android/             Android app: foreground service + WebView around the same engine
internal/scan/       engine: config parsing, anti-DPI forwarder, chain builder, browser region check
internal/ui/         single-page UI and fonts (shared by every build)
internal/buildinfo/  version stamped at release time
VERSION              the version the release workflow publishes
```

## Disclaimer
This tool measures connectivity and regional availability. Use it in accordance with the laws of your country and the terms of the services you access.
Free configs are run by unknown third parties; do not send sensitive data through them. No license has been chosen yet.
