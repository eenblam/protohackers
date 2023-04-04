package main

import (
	"io"
	"log"
	"net"
	"sync"
)

func main() {
	//addr := "127.0.0.1:9999"
	addr := net.TCPAddr{
		net.ParseIP("127.0.0.1"),
		9999,
		""}

	tests := []string{
		"asdf\nfdsa",
		"ÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏ",
		"",
		"The quick brown fox jumped over the lazy dog.",
		"!!!!!!!!!!!!!!!!!!!!!!!!!!!! @@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@@",
		"@@@@@@@@@@@@@@@@@@@@@@@@@@@@ #####################################",
		"############################ $$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$",
		"$$$$$$$$$$$$$$$$$$$$$$$$$$$$ %%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%%",
		"%%%%%%%%%%%%%%%%%%%%%%%%%%%% ^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^",
		"^^^^^^^^^^^^^^^^^^^^^^^^^^^^ &&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&&",
		"&&&&&&&&&&&&&&&&&&&&&&&&&&&& *************************************",
		"**************************** (((((((((((((((((((((((((((((((((((((",
	}

	var wg sync.WaitGroup
	for i, t := range tests {
		wg.Add(1)
		go func(i int, t string) {
			defer wg.Done()
			//fmt.Printf("Length: %d Test: %s\n", len(t), t)
			conn, err := net.DialTCP("tcp", nil, &addr)
			if err != nil {
				log.Printf("TEST %d: couldn't dial TCP: %s\n", i, err)
				return
			}
			defer conn.Close()

			// Write the test string 10000 times, just to have some concurrent load
			times := 10000
			written := 0
			for j := 0; j < times; j++ {
				w, err := conn.Write([]byte(t))
				if err != nil {
					log.Printf("TEST %d: error writing to conn: %s\n", i, err)
				}
				written += w
			}
			err = conn.CloseWrite()
			if err != nil {
				log.Printf("TEST %d: failed to close write: %s\n", i, err)
			}

			// Read and compare one byte at a time
			var buf [1]byte
			read := 0
			for {
				r, err := conn.Read(buf[:])
				//TODO Test that conn is closed with brief timeout
				if err != nil {
					if err == io.EOF {
						log.Printf("TEST %d: received EOF from server\n", i)
						conn.Close()
						break
					}
					log.Printf("TEST %d: failed to read: %s\n", i, err)
				}
				read += r
				got := buf[0]
				exp_index := (read - 1) % len(t)
				expected := t[exp_index]
				if got != expected {
					log.Printf("TEST %d: got %b expected %b at byte %d\n", i, got, expected, read)
					break
				}
			}
			// Test total is correct
			if read != len(t)*times {
				log.Printf("TEST %d: expected %d bytes, got %d\n", i, len(t)*times, read)
			}
		}(i, t)
	}
	wg.Wait()
}
