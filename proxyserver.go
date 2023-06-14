package frontstep

import (
	"context"
	"log"
	"net"
	"net/http"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

type ProxyServer struct {
	// Where we're sending UDP
	RemoteUDPAddr string
	// Where we're serving HTTP/WebSocket
	LocalWSAddr string
}

func (ps ProxyServer) Run(ctx context.Context) {
	// Get a websocket connection
	//TODO fix ip/port
	//TODO add cancel context
	log.Println("PROXYSERVER: Running")
	http.ListenAndServe(ps.LocalWSAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("PROXYSERVER:HTTP:GOT: URL %s", r.URL)
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("Couldn't upgrade request from %s: %s", conn.RemoteAddr().String(), err)
			return
		}
		go ps.handle(ctx, conn)
	}))
}

// TODO get IP/host and port from request!
func (ps ProxyServer) handle(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	//TODO logger

	// Get a "connected" UDP conn
	log.Printf("PROXYSERVER:UDP:DIAL: Dialing %s", ps.RemoteUDPAddr)
	netConn, err := net.Dial("udp", ps.RemoteUDPAddr)
	if err != nil {
		log.Printf("Error dialing remote %s: %s", ps.RemoteUDPAddr, err)
	}
	udpConn := netConn.(*net.UDPConn)

	// Get a websocket message? Forward bytes to client over UDP
	go func() {
		for {
			msg, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				//TODO reply with error? Seems like no, since we want to just be a lossy conn
				log.Printf("PROXYSERVER:WS:GOT:ERROR: %s", err)
			}
			//log.Printf("PROXYSERVER:WS:GOT: OP: %s MSG: %s", op, msg)
			log.Printf("PROXYSERVER:WS:GOT: OP: %#v", op)
			if op == ws.OpClose {
				//TODO Write any remaining data?
				cancel()
				return
			}
			// Unwrap packet, confirm long header (and address?)
			log.Printf("PROXYSERVER:UDP:SEND: Sending len=%d", len(msg))
			_, err = udpConn.Write([]byte(msg))
			if err != nil {
				log.Printf("PROXYSERVER:UDP:SEND:ERROR: Couldn't write to stream: %s", err)
				return
			}
			select {
			case <-ctx.Done():
				return
			default:
			}
		}
	}()

	// Get a UDP datagram? Forward on websocket
	//TODO make sure this is instead the maximum size of a quic message
	buf := make([]byte, 4000)
	for {
		buf = buf[:cap(buf)]
		// Get packet
		log.Println("PROXYSERVER:UDP:READ: Reading")
		//n, err := io.ReadFull(udpConn, buf)
		n, _, err := udpConn.ReadFrom(buf)
		if err != nil {
			log.Printf("PROXYSERVER:UDP:READ:ERROR: %s", err)
			continue
		}
		// Forward n bytes
		buf = buf[:n] // Don't send more than we read
		log.Printf("PROXYSERVER:WS:WRITE: len=%d", len(buf))
		err = wsutil.WriteServerMessage(conn, ws.OpBinary, buf[:n])
		if err != nil {
			log.Printf("PROXYSERVER:WS:WRITE:ERROR: %s", err)
			continue
		}
	}
}
