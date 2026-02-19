# HKA 2FA Proxy

A simple proxy to internal access of the HKA IT services by providing the OTP Base32 secret. 


[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
<br>

## Setup
Download the [latest release](https://github.com/MatthiasHarzer/hka-2fa-proxy/releases) and add the executable to your `PATH`.

## Usage
1. Start the proxy with `hka-2fa-proxy run -u <rz-username> -s <otp-secret> [-p <port>]`.
	 - The `-u` / `--username` flag is used to specify the RZ username.
	 - The `-s` / `--secret` flag is used to specify the OTP secret (Base32 encoded).
	 - The `-p` / `--port` flag is optional and specifies the port to listen on (default is 8080).
   - The `-t` / `--target` flag is optional and specifies the target URL to proxy to (default is `https://owa.h-ka.de`). See the [confirmed working URLs](#confirmed-working-urls) section below for more details.
   - The `--skip-initial-auth` flag is optional and specifies whether the initial authentication should be skipped. This can be useful when orchestrating multiple proxies which would invalidate each other's first 2FA code.
   - The `--auth-key` flag is optional and specifies a key that must be provided in the URL to access the proxy. See the [security considerations](#security-considerations) section below for more details.
2. To use the proxy, replace the host of the URL with the host of the proxy. Everything after the host remains unchanged. This means that if you want to access `https://owa.h-ka.de/owa/calendar/...`, you would replace `owa.h-ka.de` with `localhost:8080` (or whatever host and port your proxy is running on).


### Confirmed working URLs
- `https://owa.h-ka.de`: The webmail interface of the HKA. This is the primary use case for this proxy, as it allows you to access your email without needing to enter an OTP every time.
- `https://qis-extern.hs-karlsruhe.de`: The QIS portal of the HKA. 

Other URLs may work but have not been tested yet. If you want to use the proxy with a different URL, you can specify it with the `-t` / `--target` flag when starting the proxy.

### Security considerations
To prevent anyone from accessing the proxy, thus generating OTP codes and stressing the HKA's authentication servers, you can use the `--auth-key` flag to specify a key that must be provided in the URL to access the proxy. 

The key is provided in the first two URL path segments using `/_/<auth-key>/...`. For example, if you start the proxy with `hka-2fa-proxy run -u <rz-username> -s <otp-secret> --auth-key mysecretkey`, you would access the OWA at `http://localhost:8080/_/mysecretkey/owa/calendar/...`. Any request that does not include the correct key in the URL will be rejected with a 401 Unauthorized response.

> Note: This is an experimental feature and may not work correctly since rewriting the URL path is not as straightforward as it seems.

> [!WARNING]
> The `--auth-key` value is embedded in the URL path (e.g. `/_/<auth-key>/...`), which means it will appear in browser history, server/access logs, and `Referer` headers sent to external sites. Treat it as a convenience measure rather than a strong security boundary; do not reuse it as a sensitive password.

## Example Docker Compose configuration
This is an example `docker-compose.yml` file that sets up two proxies, one for the OWA and one for the QIS portal. Make sure to replace the OTP secrets with your own.
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
> Note: The `--skip-initial-auth` flag is used in this example to prevent the proxies from invalidating each other's first 2FA code.
