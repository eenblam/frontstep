# FRONTstep

FRONTstep is an attempt to incorporate a classic censorship circumvention strategy (domain fronting)
with one that was recently published (QUICstep.)
This is an educational project that is **not** intended for use against an actual adversary.

## Motivation

**Problem:** censors might attack QUIC longheader packets to certain destinations to force a downgrade to TLS/TCP..

**Solution:** tunnel the longheader packets (and DNS) for censorship resistance, but not shortheader packets for performance.
This is the [QUICstep](https://arxiv.org/abs/2304.01073) strategy,
first implemented [here](https://github.com/inspire-group/quicstep/blob/main/client/wg0.conf).

**Problem:** a Wireguard tunnel might be difficult to obtain in a censored environment.

**FRONTstep solution:** push UDP packets over a websocket tunnel obscured by domain fronting.

Is this a good idea? No. A lot of bright folks have put a lot of thought into Tor's pluggable transports. Use that.

Is it a fun idea? I think so.

## Current status

* [x] Provide a "UDP client" that implements the UDPConn interface to a websocket proxy
* [x] Provide a remote WebSocket-to-UDP proxy server
* [x] Implement an end-to-end example with a UDP echo server
* [ ] Add domain fronting support to the client
* [ ] Provide an external interface
    * Doesn't work to provide a localhost UDP proxy, since there's no way to identify the proxied destination.
    * Could instead provide a TUN interface similar to [taptun](https://github.com/pkg/taptun)
* [ ] Fun challenge: incorporate quic-go directly. Requires resolving [quic-go #3891](https://github.com/quic-go/quic-go/issues/3891).
* [ ] Set DF bit to prevent fragmentation of outgoing UDP packets

## Setup

Prereq: install Go >= 1.20. The `go run` below should install the `gobwas` packages needed for websocket support.

This will run the current echo server example end-to-end:

```bash
git clone https://github.com/eenblam/frontstep
cd frontstep/examples/echo
go run .
```

## Simple Example

`examples/echo/echoserver.go` will:

* start a UDP echo server that replies to `$STRING` with `$STRING $REMOTE_ADDRESS`
* start a `ProxyServer` that accepts websocket connections to `/?address=$ADDRESS`,
then forwards any websocket messages over UDP to `$ADDRESS` (in `hostname:port` format.)
* start a `ProxyClient` with two connections: a connected UDP socket to the echo server, and a websocket connection to the proxy server.
    * The `ProxyClient` is also configured with a function for routing any UDP messages written to it, which returns `true` only for the string "hello", otherwise `false`.
* write packets "hello" and "hi" to the proxy client, and listen for replies on both of its connections.

(This is a first step towards implementing QUICstep:
routing long header packets for connection negotiation over a proxy,
and then sending short header (data) packets over a UDP socket.)

There can be a lot of output, but filtering for the MAIN function of the program, we see:

```bash
b@tincan:~/b/gh/eenblam/frontstep/examples/echo$ go run . 2>&1 | grep MAIN
2023/06/15 20:53:38 ECHOSERVER:MAIN:ReadMsgUDP: Got 'hello 127.0.0.1:50557'
2023/06/15 20:53:38 ECHOSERVER:MAIN:ReadMsgUDP: Got 'hi 127.0.0.1:56729'
```

Here, we see that the echo server received "hello" and "hi" over different connections,
as expected.

(This doesn't include the domain fronting aspect!
It's just meant to provide a proof of concept for the QUICstep part over localhost.)