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
2. To use the proxy, replace the host of the URL with the host of the proxy. Everything after the host remains unchanged. This means that if you want to access `https://owa.h-ka.de/owa/calendar/...`, you would replace `owa.h-ka.de` with `localhost:8080` (or whatever host and port your proxy is running on).


### Confirmed working URLs
- `https://owa.h-ka.de`: The webmail interface of the HKA. This is the primary use case for this proxy, as it allows you to access your email without needing to enter an OTP every time.
- `https://qis-extern.hs-karlsruhe.de`: The QIS portal of the HKA. 

Other URLs may work but have not been tested yet. If you want to use the proxy with a different URL, you can specify it with the `-t` / `--target` flag when starting the proxy.
