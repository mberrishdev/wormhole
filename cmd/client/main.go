package main

import (
	"flag"
	"io"
	"log"
	"net"
	"sync"

	"golang.org/x/crypto/ssh"

	"github.com/mberrishdev/wormhole/internal/dashboard"
)

func main() {
	server := flag.String("server", "13.62.136.105:2222", "SSH server address")
	password := flag.String("password", "secret123", "SSH password")
	local := flag.String("local", "localhost:3000", "local service address")
	flag.Parse()

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

	ok, reply, err := conn.SendRequest("forward", true, nil)
	log.Printf("ok=%v reply=%s err=%v", ok, string(reply), err)
	if !ok || err != nil {
		log.Fatal("forward request rejected")
	}

	token := string(reply)

	dashboard.EnsureRunning(":4040")
	dashboard.Register(token, *local, "https://"+token+".wormhole.mberrishdev.me")
	defer dashboard.Unregister(token)

	log.Printf("tunnel is live at: https://%s.wormhole.mberrishdev.me", token)
	log.Println("dashboard: http://localhost:4040")

	channels := conn.HandleChannelOpen("tunnel")

	for newChannel := range channels {
		ip := string(newChannel.ExtraData())
		go handleChannel(newChannel, *local, token, ip)
	}
}

func handleChannel(newChannel ssh.NewChannel, localAddr, token, ip string) {
	channel, requests, err := newChannel.Accept()
	if err != nil {
		log.Println("accept channel error:", err)
		return
	}
	defer channel.Close()

	go ssh.DiscardRequests(requests)

	localConn, err := net.Dial("tcp", localAddr)
	if err != nil {
		log.Println("local dial error:", err)
		return
	}
	defer localConn.Close()

	dashboard.Connect(token, ip)
	defer dashboard.Disconnect(token, ip)

	log.Println("new tunnel connection")

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
