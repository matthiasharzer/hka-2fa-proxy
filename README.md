# HKA 2FA Proxy

A lightweight proxy that handles 2FA authentication for HKA IT services automatically. It intercepts requests, generates a fresh OTP from your Base32 secret, and forwards traffic to the target service — so you no longer need to enter a one-time password manually on every request.

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
<br>

## Setup

### Docker (recommended)

The easiest way to run the proxy is with Docker. A pre-built image is available on the [GitHub Container Registry](https://ghcr.io/matthiasharzer/hka-2fa-proxy).

#### Docker Compose

Create a `docker-compose.yml` file and start it with `docker compose up -d`. The example below sets up two proxies — one for OWA and one for the QIS portal. Replace the credentials with your own.

```yaml
services:
  owa-proxy:
    image: ghcr.io/matthiasharzer/hka-2fa-proxy:latest
    container_name: owa-proxy
    restart: unless-stopped
    ports:
      - "8080:8080"
    command: run -p 8080 -u <your-rz-username> -s <your-base32-encoded-2fa-secret> -t "https://owa.h-ka.de" --skip-initial-auth

  qis-proxy:
    image: ghcr.io/matthiasharzer/hka-2fa-proxy:latest
    container_name: qis-proxy
    restart: unless-stopped
    ports:
      - "8081:8080"
    command: run -p 8080 -u <your-rz-username> -s <your-base32-encoded-2fa-secret> -t "https://qis-extern.hs-karlsruhe.de" --skip-initial-auth
```

> [!NOTE]
> The `--skip-initial-auth` flag prevents multiple proxies from invalidating each other's first 2FA code on startup.

#### Docker CLI

```bash
docker run -d \
  --name owa-proxy \
  --restart unless-stopped \
  -p 8080:8080 \
  ghcr.io/matthiasharzer/hka-2fa-proxy:latest \
  run -p 8080 -u <your-rz-username> -s <your-base32-encoded-2fa-secret>
```

### Binary

Download the [latest release](https://github.com/MatthiasHarzer/hka-2fa-proxy/releases) for your platform and add the executable to your `PATH`.

## Usage

Start the proxy with:

```bash
hka-2fa-proxy run -u <rz-username> -s <otp-secret> [-p <port>]
```

| Flag | Required | Default | Description |
|---|---|---|---|
| `-u` / `--username` | ✅ | — | Your RZ username |
| `-s` / `--secret` | ✅ | — | Your OTP secret (Base32 encoded) |
| `-p` / `--port` | ❌ | `8080` | Port to listen on |
| `-t` / `--target` | ❌ | `https://owa.h-ka.de` | Target URL to proxy to (see [confirmed working URLs](#confirmed-working-urls)) |
| `--skip-initial-auth` | ❌ | `false` | Skip the initial authentication. Useful when running multiple proxies to avoid invalidating each other's first 2FA code |
| `--auth-key` | ❌ | — | Restrict proxy access with a secret key embedded in the request URL (see [security considerations](#security-considerations)) |

Once the proxy is running, replace the hostname in any URL with `localhost:<port>`. Everything after the host remains unchanged.

**Example:** `https://owa.h-ka.de/owa/calendar/...` → `http://localhost:8080/owa/calendar/...`

**When `--auth-key` is set**, the key must be included in the URL path as `/_/<auth-key>/`:

`https://owa.h-ka.de/owa/calendar/...` → `http://localhost:8080/_/mysecretkey/owa/calendar/...`

Any request without the correct key is rejected with `401 Unauthorized`.

### Confirmed working URLs

- `https://owa.h-ka.de` — The HKA webmail interface (OWA). The primary use case for this proxy.
- `https://qis-extern.hs-karlsruhe.de` — The HKA QIS portal.

Other URLs may work but have not been tested yet. Use the `-t` / `--target` flag to specify a different URL.

### Security considerations

To prevent unauthorized access to the proxy — which would trigger OTP code generation and put unnecessary load on the HKA authentication servers — use the `--auth-key` flag to require a secret key in every request URL.

The key must consist only of alphanumeric characters, hyphens, and underscores (e.g. `my-secret_key123`). The proxy will refuse to start if an invalid key is supplied.

| Without `--auth-key` | With `--auth-key mysecretkey` |
|---|---|
| `http://localhost:8080/owa/calendar/...` | `http://localhost:8080/_/mysecretkey/owa/calendar/...` |

> [!NOTE]
> This is an experimental feature and may not work correctly since rewriting the URL path is not as straightforward as it seems.

> [!WARNING]
> The `--auth-key` value is embedded in the URL path (e.g. `/_/<auth-key>/...`), which means it will appear in browser history, server/access logs, and `Referer` headers sent to external sites. Treat it as a convenience measure rather than a strong security boundary; do not reuse it as a sensitive password.
