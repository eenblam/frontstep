# FRONTstep

FRONTstep is an attempt to incorporate a classic censorship circumvention strategy (domain fronting)
with one that was recently published (QUICstep.)
However, this is an educational project,
and it is **not** intended for actual use.

## Motivation

**Problem:** Censors might attack QUIC longheader packets to certain destinations to force a downgrade to TLS/TCP..

**Solution:** tunnel the longheader packets (and DNS) for censorship resistance, but not shortheader packets for performance.
This is the [QUICstep](https://arxiv.org/abs/2304.01073) strategy,
first implemented [here](https://github.com/inspire-group/quicstep/blob/main/client/wg0.conf).

**Problem:** a Wireguard tunnel might also be difficult to obtain in a censored environment.

**FRONTstep solution:** push UDP packets over a websocket tunnel obscured by domain fronting.

Is this a good idea? No. A lot of bright folks have put a lot of thought into Tor's pluggable transports. Use that.

Is it a fun idea? Sure.

## Current status

* [x] Provide a "UDP client" that implements the UDPConn interface to a websocket proxy
* [x] Provide a remote WebSocket-to-UDP proxy server
* [x] Implement an end-to-end example with a UDP echo server
* [ ] Add domain fronting support to the client
* [ ] Provide an external interface
    * Doesn't work to provide a localhost UDP proxy, since there's no way to identify the proxied destination.
    * Could instead provide a TUN interface similar to [taptun](https://github.com/pkg/taptun)
* [ ] Fun challenge: incorporate quic-go. Requires resolving [quic-go #3891](https://github.com/quic-go/quic-go/issues/3891).
* [ ] Set DF bit to prevent fragmentation of outgoing UDP packets

## Usage

Prereq: install Go >= 1.20. The `go run` below should install the `gobwas` packages needed for websocket support.

This will run the current echo server example end-to-end:

```bash
git clone https://github.com/eenblam/frontstep
cd frontstep/examples/echo
go run .
```