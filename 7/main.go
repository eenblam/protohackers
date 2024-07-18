package linereversal

import (
	"context"
	"fmt"
	"log"
	"net"
)

// "LRCP messages must be smaller than 1000 bytes."
const maxMessageSize = 999

var (
	localAddr  = "0.0.0.0"
	localPort  = 4321
	maxWorkers = 10
)

func main() {

	UDPLocalAddr := &net.UDPAddr{
		IP:   net.ParseIP(localAddr),
		Port: localPort,
		Zone: "",
	}
	srv, err := net.ListenUDP("udp", UDPLocalAddr)
	if err != nil {
		log.Fatalf(`error listening on %s:%d: %s`, localAddr, localPort, err)
	}
	log.Printf(`listening on %s:%d`, localAddr, localPort)

	ctx := context.Background()
	for i := 0; i < maxWorkers; i++ {
		go worker(ctx, srv, fmt.Sprint(i))
	}
}

func worker(ctx context.Context, srv *net.UDPConn, id string) {
	buf := make([]byte, maxMessageSize)
	for {
		select {
		case <-ctx.Done():
			return
		default:
			n, addr, err := srv.ReadFrom(buf)
			if err != nil {
				log.Fatalf(`error reading from %s: %s`, addr, err)
			}
			request := string(buf[:n])
			log.Printf(`got %d bytes from %s: [%s]`, n, addr.String(), request)
		}
	}

}
