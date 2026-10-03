# Security and privacy notes

## What the app handles

| Item | Where it lives | Leaves the machine? |
|---|---|---|
| Google password | never seen by the app (you type it into your own browser) | no |
| Google session cookies | memory only | only to `*.google.com`, through your own proxy chain over TLS |
| Isolated browser profile | `<user config dir>/GeminiConfigChecker/browser-profile` | no; deleted by *Remove account from app* |
| Base config and settings | the UI's local storage, only when "remember" is ticked | no |
| Free configs | downloaded from the list URL you choose | n/a |

The app does **not** read, copy or decrypt cookies from your everyday browser profile. It uses its own isolated profile on purpose.

## Threat model and limits

- The embedded UI server (web build) has **no authentication**. It binds to `127.0.0.1` by default and rejects foreign `Host` headers and non-JSON POSTs. If you bind it to a public address, put your own auth or a tunnel in front of it.
- Free configs are operated by unknown third parties. TLS to Google is end-to-end, so an operator sees destinations and timing, not content. Do not push sensitive traffic through them.
- Requests reach Google from many different IPs; Google may raise a "suspicious sign-in" notice. Use a secondary Google account.
- Release binaries are unsigned. Verify downloads with `SHA256SUMS.txt`.

## Reporting a vulnerability

Please do not open a public issue for a security problem. Contact the maintainer privately via <https://t.me/MatinSenPaii> and include
steps to reproduce. You will get an acknowledgement as soon as possible.
