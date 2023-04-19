package main

import (
	"fmt"
	"log"
	"net"
	"strings"
)

const port = 3334

func main() {
	UDPAddr := &net.UDPAddr{
		net.ParseIP("0.0.0.0"),
		port,
		"",
	}
	srv, err := net.ListenUDP("udp", UDPAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer srv.Close()
	log.Printf("Listening on %d", port)

	// "All requests and responses must be shorter than 1000 bytes."
	maxSize := 999
	buf := make([]byte, maxSize)
	data := make(map[string]string)

	for {
		n, addr, err := srv.ReadFrom(buf)
		if err != nil {
			//TODO what does this mean? Are there interesting errors to check for?
			// Do we even want to stop?
			log.Fatal(err)
			return
		}

		request := string(buf[:n])

		log.Printf("Got %d bytes from %s: [%s]", n, addr.String(), request)
		key, value, isInsert := strings.Cut(request, "=")
		if isInsert {
			// Update data, no reply
			data[key] = value
			log.Printf("Set %s=%s", key, value)
		} else { // Query
			var val string
			if request == "version" {
				val = "0.0.1"
			} else {
				// Requirement: missing value can be `<key>=` or no response
				// Since go map sets missing key = empty string, we just ignore this case.
				val = data[key]
			}
			reply := fmt.Sprintf(`%s=%s`, request, val)
			log.Printf("Reply: [%s]", reply)
			n, err = srv.WriteTo([]byte(reply), addr)
			if err != nil {
				log.Fatalf("??? %s", err)
			}
		}
	}
}
