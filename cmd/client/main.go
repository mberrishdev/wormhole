package main

import (
	"io"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	config := &ssh.ClientConfig{
		User: "wormhole",
		Auth: []ssh.AuthMethod{
			ssh.Password("secret123"),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", "localhost:2222", config)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	log.Println("connected to server")

	// 1. ask server to expose :8888
	ok, _, err := conn.SendRequest("forward", true, []byte("8888"))
	if !ok || err != nil {
		log.Fatal("forward request rejected")
	}

	log.Println("server is now forwarding :8888 to us")

	// 2. handle incoming channels from server
	channels := conn.HandleChannelOpen("tunnel")

	for newChannel := range channels {
		go handleChannel(newChannel)
	}
}

func handleChannel(newChannel ssh.NewChannel) {
	// 3a. accept incoming SSH channel
	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Println("accept channel error:", err)
		return
	}
	defer channel.Close()

	// 3b. discard SSH requests
	go ssh.DiscardRequests(requests)

	// 3c. connect to local service
	localConn, err := net.Dial("tcp", "localhost:3000")
	if err != nil {
		log.Println("local dial error:", err)
		return
	}
	defer localConn.Close()

	log.Println("new tunnel connection")

	// 3d. pipe traffic both directions
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		io.Copy(channel, localConn)
	}()

	go func() {
		defer wg.Done()
		io.Copy(localConn, channel)
	}()

	wg.Wait()
}
