package main

import (
	"fmt"
	"io"
	"log"
	"net"
	"sync"
)

func main() {

	ln, err := net.Listen("tcp", ":9000")
	if err != nil {
		log.Fatal("Error on listen")
	}

	fmt.Println("Listening on :9000")

	for {
		connA, err := ln.Accept()

		if err != nil {
			log.Fatal("Error on Accept")
		}

		connB, errDial := net.Dial("tcp", ":3000")

		if errDial != nil {
			connA.Close()
			continue
		}

		go pipe(connA, connB)
	}
}

func pipe(connA net.Conn, connB net.Conn) {

	defer connA.Close()
	defer connB.Close()

	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(connA, connB)
	}()

	go func() {
		defer wg.Done()
		io.Copy(connB, connA)
	}()

	wg.Wait()
}
