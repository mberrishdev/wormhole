package main

import (
	"flag"
	"io"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	server := flag.String("server", "13.62.136.105:2222", "SSH server address")
	password := flag.String("password", "secret123", "SSH password")
	local := flag.String("local", "localhost:3000", "local service address")
	flag.Parse()

	// host, _, _ := net.SplitHostPort(*server)
	// if *token == "" {
	// 	*token = randomToken()
	// }

	config := &ssh.ClientConfig{
		User: "wormhole",
		Auth: []ssh.AuthMethod{
			ssh.Password(*password),
		},
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
	}

	conn, err := ssh.Dial("tcp", *server, config)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()

	log.Println("connected to server")

	// 1. ask server to expose :8888
	// payload := fmt.Sprintf("%s:%s", *publicPort, *token)

	ok, reply, err := conn.SendRequest("forward", true, nil)
	log.Printf("ok=%v reply=%s err=%v", ok, string(reply), err)
	if !ok || err != nil {
		log.Fatal("forward request rejected")
	}

	token := string(reply)

	log.Printf("tunnel is live at: http://%s.wormhole.mberrishdev.me", token)

	// 2. handle incoming channels from server
	channels := conn.HandleChannelOpen("tunnel")

	for newChannel := range channels {
		go handleChannel(newChannel, *local)
	}
}

func handleChannel(newChannel ssh.NewChannel, localAddr string) {
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
	localConn, err := net.Dial("tcp", localAddr)
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
