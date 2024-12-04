package main

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"slices"
)

var (
	localAddr = "0.0.0.0"
	localPort = 4321
)

func main() {
	laddr := &net.UDPAddr{
		IP:   net.ParseIP(localAddr),
		Port: localPort,
		Zone: "",
	}

	l, err := Listen(laddr)
	if err != nil {
		log.Fatalf(`error listening: %s`, err)
	}
	for {
		session := l.Accept()
		log.Printf(`accepted session [%s]`, session.Key())

		go reverseSessionHandler(session)
	}
}

// reverseSessionHandler implements the application layer by simply reading until a new line
// and then responding with a reversed copy of each line.
func reverseSessionHandler(session *Session) {
	defer session.Close()

	scanner := bufio.NewScanner(session)
	// Default token size is 64k; we might receive maxInt bytes before newline
	buf := make([]byte, maxInt)
	scanner.Buffer(buf, maxInt)
	scanner.Split(ScanLinesNoCR)

	for scanner.Scan() {
		log.Println("SCAN")
		data := scanner.Bytes()
		log.Printf(`Reverse: Session[%s] received [%d] bytes`, session.Key(), len(data))
		slices.Reverse(data)
		data = append(data, '\n')
		_, err := session.Write(data)
		if err != nil {
			log.Printf(`Reverse: Session[%s] encountered error on write: %s`, session.Key(), err)
			session.sendClose()
			break
		}
		log.Printf(`Reverse: Session[%s] sent [%d] bytes`, session.Key(), len(data))
	}
	if err := scanner.Err(); err != nil {
		log.Printf(`Reverse: Session[%s] scanner exited with error: %s`, session.Key(), err)
	}
}

// ScanLinesNoCR works like bufio.ScanLines, but it doesn't try to strip carriage return (\r 0x0D).
// This can cause several issues:
// * Simply returning the wrong data when a \r is skipped
// * Hanging on `abcd\r\nabcd\n` since it ignores the second \n in favor of an \r\n
func ScanLinesNoCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexByte(data, '\n'); i >= 0 {
		// We have a full newline-terminated line.
		return i + 1, data[0:i], nil
	}
	// If we're at EOF, we have a final, non-terminated line. Return it.
	if atEOF {
		return len(data), data, nil
	}
	// Request more data.
	return 0, nil, nil
}
