package main

import (
	"bufio"
	"fmt"
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
			log.Println(err)
			continue
		}
		data := scanner.Bytes()
		fmt.Println(data)
		slices.Reverse(data)
		data = append(data, '\n')
		_, err := session.Write(data)
		if err != nil {
			log.Printf(`error writing to session [%s]: %s`, session.Key(), err)
		}
	}
}
