package main

// This is just a small UDP echo server for demonstration purposes

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/eenblam/frontstep"
)

const echoServerAddr = "localhost:4242"

const proxyServerAddr = "localhost:3636"

const writeTimeout = 1 * time.Second

func main() {
	ctx, cancelFunc := context.WithCancel(context.Background())
	// Start UDP echo server
	go func() { echoServer(ctx, echoServerAddr) }()
	time.Sleep(time.Second)

	// Start "remote" proxy
	// This would normally be domain-fronted
	go frontstep.ProxyListenAndServe(ctx, proxyServerAddr)
	time.Sleep(time.Second)

	// Proxy "hello". Anything else goes over local UDP conn.
	shouldProxy := func(bs []byte) bool {
		if len(bs) == 5 && bytes.Equal(bs, []byte("hello")) {
			return true
		}
		return false
	}
	// Run local client
	proxyClient, err := frontstep.DialAddr(echoServerAddr, proxyServerAddr, shouldProxy)
	if err != nil {
		cancelFunc()
		panic(err)
	}
	defer proxyClient.Close()

	go proxyClient.Run(ctx)

	time.Sleep(100 * time.Microsecond)

	buf := make([]byte, frontstep.ReadBufSize)

	proxyClient.WriteMsgUDP([]byte("hello"), nil, nil)
	n, _, _, _, err := proxyClient.ReadMsgUDP(buf, nil)
	if err != nil {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP:ERROR: %s", err)
	} else {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP: Got '%s'", string(buf[:n]))
	}
	proxyClient.WriteMsgUDP([]byte("hi"), nil, nil)
	n, _, _, _, err = proxyClient.ReadMsgUDP(buf, nil)
	if err != nil {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP:ERROR: %s", err)
	} else {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP: Got '%s'", string(buf[:n]))
	}

	// Let services tear down gracefully
	cancelFunc()
	//wg.Wait()
}

// Start a UDP server that echoes all data on the first stream opened by the client
func echoServer(ctx context.Context, address string) {
	conn, err := net.ListenPacket("udp", address)
	if err != nil {
		log.Printf("ECHOSERVER:LISTEN:ERROR: %s", err)
		return
	}
	defer conn.Close()
	log.Printf("ECHOSERVER:LISTEN: listening on %s", conn.LocalAddr())

	buf := make([]byte, frontstep.ReadBufSize)

	for {
		select {
		case <-ctx.Done():
			log.Println("ECHOSERVER: cancelled")
			return
		default:
		}
		log.Println("ECHOSERVER:UDP:READ: Reading")
		n, addr, err := conn.ReadFrom(buf)
		if err != nil { //TODO check more specific error
			log.Printf("ECHOSERVER:READ:ERROR: failed to read from UDP: %s", err)
			continue
		}
		log.Printf("ECHOSERVER:UDP:GOT: from %s: '%s'", addr, string(buf[:n]))
		err = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err != nil {
			log.Printf("ECHOSERVER:ERROR: failed to set write deadline: %s", err)
			return
		}
		out := []byte(fmt.Sprintf("%s %s", string(buf[:n]), addr.String()))
		_, err = conn.WriteTo(out, addr)
		if err != nil { //TODO check more specific error
			log.Printf("ECHOSERVER:UDP:WRITE:ERROR: failed to write to UDP: %s", err)
			continue
		}
	}
}
