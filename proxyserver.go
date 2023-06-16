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
	// Where we're serving HTTP/WebSocket
	LocalWSAddr string
}

func (ps ProxyServer) Run(ctx context.Context) {
	// Get a websocket connection
	//TODO add cancel context
	log.Println("PROXYSERVER: Running")
	http.ListenAndServe(ps.LocalWSAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("PROXYSERVER:HTTP:GOT: URL %s", r.URL)
		rawAddr := r.URL.Query().Get("address")
		remoteUDPAddr, err := net.ResolveUDPAddr("udp", rawAddr)
		if err != nil || rawAddr == "" {
			// Note that rawAddr == "" won't trigger an err, have to check it separately
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		log.Printf("PROXYSERVER:HTTP: connecting %s to %s", r.RemoteAddr, remoteUDPAddr)
		conn, _, _, err := ws.UpgradeHTTP(r, w)
		if err != nil {
			log.Printf("Couldn't upgrade request from %s: %s", conn.RemoteAddr().String(), err)
			return
		}
		go ps.handle(ctx, conn, remoteUDPAddr)
	}))
}

func (ps ProxyServer) handle(ctx context.Context, conn net.Conn, remoteUDPAddr *net.UDPAddr) {
	defer conn.Close()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	//TODO logger

	// Get a "connected" UDP conn
	log.Printf("PROXYSERVER:UDP:DIAL: Dialing %s", remoteUDPAddr)
	netConn, err := net.Dial("udp", remoteUDPAddr.String())
	if err != nil {
		log.Printf("Error dialing remote %s: %s", remoteUDPAddr, err)
	}
	udpConn := netConn.(*net.UDPConn)

	// Get a websocket message? Forward bytes to client over UDP
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
			}
			msg, op, err := wsutil.ReadClientData(conn)
			if err != nil {
				// Don't sweat errors, since we want to just be a lossy conn
				log.Printf("PROXYSERVER:WS:GOT:ERROR: %s", err)
				continue
			}
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
		}
	}()

	// Get a UDP datagram? Forward on websocket
	buf := make([]byte, ReadBufSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}
		buf = buf[:cap(buf)]
		// Get packet
		log.Println("PROXYSERVER:UDP:READ: Reading")
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
