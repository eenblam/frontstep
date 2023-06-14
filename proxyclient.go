package frontstep

// Ugh I think I should just pay Fastly for websockets
// If someone can pay for Fastly, not unreasonable to also pay for websockets

import (
	//"net/http"
	"context"
	"encoding/hex"
	"errors"
	"io"
	"log"
	"net"
	"strings"
	"sync"

	"github.com/gobwas/ws"
	"github.com/gobwas/ws/wsutil"
)

// From https://github.com/quic-go/quic-go/blob/2ff71510a9c447aad7f5a574fe0f6cf715e749f1/client.go#L47
func DialAddr(addr string) (*ProxyClient, error) {
	// We replace net.ListenUDP with our own type
	netConn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, err
	}
	udpConn, ok := netConn.(*net.UDPConn)
	if !ok {
		return nil, errors.New("couldn't convert net.Conn to net.UDPConn")
	}

	pc := ProxyClient{
		UDPConn: *udpConn,
		dstAddr: addr,
		//make(chan []byte), // responseChan
		//make(chan []byte), // fwCh
		//make(chan []byte), // fwNoWaitCh
		readLocal:  make(chan MsgUDP),
		readProxy:  make(chan MsgUDP),
		writeProxy: make(chan []byte),
	}

	return &pc, nil
}

// TODO domain fronting
// TODO handle https://{proxyaddr}/?addr={addr}
type ProxyClient struct {
	net.UDPConn
	dstAddr    string
	dstUDPAddr *net.UDPAddr
	// Local reads
	readLocal chan MsgUDP
	// Read/Write proxy
	//readProxy  chan []byte
	readProxy  chan MsgUDP
	writeProxy chan []byte
}

// Response from ReadMsgUDP
// n int, oobn int, flags int, addr *net.UDPAddr, err error
type MsgUDP struct {
	bs   []byte
	addr *net.UDPAddr
}

func (pc *ProxyClient) Run(ctx context.Context) {
	// We want to cancel if proxy sends opclose
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	log.Println("PROXYCLIENT: Running")

	// net.Conn, *bufio.Reader, Handshake, error
	// Can ignore Reader, but may want to do pbufio.PutReader(buf) for memory
	proxyWSAddr := "ws://" + pc.dstAddr
	conn, _, _, err := ws.Dial(ctx, proxyWSAddr)
	if err != nil {
		log.Fatalf("PROXYCLIENT:WS:DIAL: %s", err)
	}
	defer conn.Close()
	log.Printf("PROXYCLIENT:WS: dialed %s", proxyWSAddr)

	// Handle messages received locally
	// This is for the specific case in which we would merge incoming data streams
	// from both a WebSocket proxy (for QUIC long header packets)
	// and a local UDP connection (for QUIC short header packets)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		b := make([]byte, 0)
		oob := make([]byte, 0)
		for {
			select {
			case <-ctx.Done():
				log.Printf("PROXYCLIENT:UDP:READ: Context done. Closing.")
				return
			default:
			}
			_, _, _, udpAddr, err := pc.UDPConn.ReadMsgUDP(b, oob)
			if err != nil {
				log.Printf("PROXYCLIENT:UDP:READ:ERROR: %s", err)
				continue
			}
			//TODO call cancel if UDPConn is closed!

			pc.readLocal <- MsgUDP{b, udpAddr}
		}
	}()

	// Handle messages from proxy server
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				log.Printf("PROXYCLIENT:WS:READ: Context done. Closing.")
				return
			default:
			}
			//TODO this may interfere with our writes
			// by updating the writer with control frames from reads
			data, op, err := wsutil.ReadData(conn, ws.StateClientSide)
			if err != nil && err != io.EOF {
				//TODO handle error
				log.Printf("PROXYCLIENT:WS:READ:ERROR: %s", err)
				continue
			}
			if err == io.EOF {
				log.Println("PROXYCLIENT:WS:READ: Received EOF")
				return
			}
			if op == ws.OpClose {
				//TODO is this what I want to do?
				//TODO probably want to cancel context so writes stop
				// or send to a Closed channel
				log.Println("PROXYCLIENT:WS:GOT: Got OpClose from proxy. Closing.")
				cancel()
				//return io.EOF
				return
			}
			if op != ws.OpBinary {
				//TODO maybe hexdump?
				log.Printf("PROXYCLIENT:WS:GOT:ERROR: Expected OpBinary, got %#v with data: %s", op, string(data))
				continue
			}
			//TODO Is this just my raw bytes?
			pc.readProxy <- MsgUDP{data, pc.dstUDPAddr}
		}
	}()

	// Handle websocket writes to proxy
	wg.Add(1)
	go func() {
		defer wg.Done()
		log.Println("PROXYCLIENT: Handling writes")
		for {
			// Maybe instead use a Closed channel, then goto DIAL if we need to reconnect?
			select {
			case <-ctx.Done():
				log.Printf("PROXYCLIENT:WS:WRITE: Context done. Closing.")
				return
			default:
			}
			// Get outbound packet
			data := <-pc.writeProxy
			log.Println("PROXYCLIENT:WS:WRITE: writing")
			err = wsutil.WriteClientBinary(conn, data)
			if err != nil {
				log.Printf("PROXYCLIENT:WS:WRITE:ERROR: %s", err)
			}
		}
	}()

	// Allow writes to finish before allowing conn to close
	wg.Wait()
	log.Printf("PROXYCLIENT: all workers done. Closing conn to %s", conn.RemoteAddr())
}

func (pc *ProxyClient) WriteTo(bs []byte, addr net.Addr) (int, error) {
	//TODO error if addr doesn't match pc.dstAddr
	log.Printf("PROXYCLIENT:UDP:WriteTo: %v", addr)
	log.Print("PROXYCLIENT:UDP:WriteTo: Packet:\n\t" + strings.ReplaceAll(hex.Dump(bs), "\n", "\n\t"))
	pc.writeProxy <- bs
	return len(bs), nil
	//TODO condition for writing to underlying stream instead
	//return pc.UDPConn.WriteTo(p, addr)
}

func (pc *ProxyClient) WriteMsgUDP(bs, oob []byte, addr *net.UDPAddr) (int, int, error) {
	//TODO error if addr doesn't match pc.dstAddr
	log.Printf("PROXYCLIENT:UDP:WriteMsgUDP: %v", addr)
	log.Print("PROXYCLIENT:UDP:WriteMsgUDP: Packet:\n\t" + strings.ReplaceAll(hex.Dump(bs), "\n", "\n\t"))
	pc.writeProxy <- bs
	//TODO should we lie about sending oob bytes?
	return len(bs), len(oob), nil
	//TODO condition for writing to underlying stream instead
	//return pc.UDPConn.WriteMsgUDP(p, oob, addr)
}

// TODO not implementing this yet, basically same as ReadMsgUDP
func (pc *ProxyClient) ReadFrom(bs []byte) (int, net.Addr, error) {
	log.Println("PROXYCLIENT:ReadFrom: Called")
	return pc.UDPConn.ReadFrom(bs)
}

// n int, oobn int, flags int, addr *net.UDPAddr, err error
func (pc *ProxyClient) ReadMsgUDP(bs, oob []byte) (int, int, int, *net.UDPAddr, error) {
	// Check read channel, then try reading
	log.Println("PROXYCLIENT:ReadMsgUDP: Begin")
	defer log.Println("PROXYCLIENT:ReadMsgUDP: End")
	for {
		// Read UDP messages, prioritizing remote proxy reads over local proxy
		// (Need a nested select for prioritization. Maybe doesn't matter?)
		select {
		case msgUDP := <-pc.readProxy:
			//TODO only write up to length of b instead! Do I need to cache the extra?
			n := copy(bs, msgUDP.bs)
			log.Printf("PROXYCLIENT:UDP:ReadMsgUDP:FromProxy: %s", bs)
			return n, 0, 0, msgUDP.addr, nil
		default:
			select {
			case msgUDP := <-pc.readLocal:
				//TODO only write up to length of b instead! Do I need to cache the extra?
				n := copy(bs, msgUDP.bs)
				log.Printf("PROXYCLIENT:UDP:ReadMsgUDP:FromLocal: %s", bs)
				return n, 0, 0, msgUDP.addr, nil
			default: // don't block
			}
		}

	}
}
