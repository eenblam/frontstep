package main

// This is just a small UDP echo server for demonstration purposes

import (
	"context"
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
	ps := &frontstep.ProxyServer{
		LocalWSAddr: proxyServerAddr,
	}

	go ps.Run(ctx)
	time.Sleep(time.Second)

	// Run local client
	// For this example, we aren't interested in sending anything over UDP, only WebSockets.
	shouldProxy := func(_ []byte) bool { return true }
	proxyClient, err := frontstep.DialAddr(echoServerAddr, proxyServerAddr, shouldProxy)
	if err != nil {
		cancelFunc()
		panic(err)
	}
	defer proxyClient.Close()

	go proxyClient.Run(ctx)

	time.Sleep(100 * time.Microsecond)

	proxyClient.WriteTo([]byte("hello"), proxyClient.LocalAddr())
	buf := make([]byte, frontstep.ReadBufSize)
	n, _, _, _, err := proxyClient.ReadMsgUDP(buf, nil)
	if err != nil {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP:ERROR: %s", err)
	} else {
		log.Printf("ECHOSERVER:MAIN:ReadMsgUDP: Got %s", string(buf[:n]))
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

	// Our simple echo protocol is only going to support payloads up to 1024 bytes in size
	buf := make([]byte, 1024)

	for {
		select {
		case <-ctx.Done():
			log.Println("ECHOSERVER: cancelled")
			return
		default:
		}
		n, addr, err := conn.ReadFrom(buf)
		if err != nil { //TODO check more specific error
			log.Printf("ECHOSERVER:READ:ERROR: failed to read from UDP: %s", err)
			continue
		}
		log.Printf("ECHOSERVER:GOT: from %s: '%s'", addr, string(buf))
		err = conn.SetWriteDeadline(time.Now().Add(writeTimeout))
		if err != nil {
			log.Printf("ECHOSERVER:ERROR: failed to set write deadline: %s", err)
			return
		}
		_, err = conn.WriteTo(buf[:n], addr)
		if err != nil { //TODO check more specific error
			log.Printf("ECHOSERVER:WRITE:ERROR: failed to write to UDP: %s", err)
			continue
		}
	}
}
