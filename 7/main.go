package main

import (
	"bufio"
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

		go reverseSessionHandler(session)
	}
}

func reverseSessionHandler(session *Session) {
	scanner := bufio.NewScanner(session)
	for scanner.Scan() {
		if err := scanner.Err(); err != nil {
			log.Printf(`Session[%s] error reading: %s`, session.Key(), err)
			continue
		}
		data := scanner.Bytes()
		log.Printf(`Session[%s] received message: %s`, session.Key(), data)
		slices.Reverse(data)
		data = append(data, '\n')
		_, err := session.Write(data)
		if err != nil {
			log.Printf(`error writing to session [%s]: %s`, session.Key(), err)
		}
		log.Printf(`Session[%s] sent message: %s`, session.Key(), data)
	}
}
