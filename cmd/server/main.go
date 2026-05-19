package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	signer, err := loadOrGenerateKey("server.key")
	if err != nil {
		log.Fatal(err)
	}

	// build SSH server config
	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) == "secret123" {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected")
		},
	}
	config.AddHostKey(signer)

	// listen for raw tcp connection

	ln, err := net.Listen("tcp", ":2222")

	if err != nil {
		log.Fatal(err)
	}

	defer ln.Close()

	log.Println("SSH server listening on :2222")

	// accept raw TCP connection
	conn, err := ln.Accept()
	if err != nil {
		log.Fatal(err)
	}

	// upgrade TCP -> SSH
	sshConn, _, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Fatal(err)
	}

	defer sshConn.Close()

	log.Println("tunnel client connected:", sshConn.RemoteAddr())

	for req := range reqs {
		if req.Type == "forward" {
			port := string(req.Payload)
			log.Println("client wants to forward port:", port)
			go listenPublic(":"+port, sshConn)
			req.Reply(true, nil)
		}
	}
}

func loadOrGenerateKey(path string) (ssh.Signer, error) {
	byt, err := os.ReadFile(path)
	if err == nil {
		signer, err := ssh.ParsePrivateKey(byt)
		if err != nil {
			return nil, err
		}

		return signer, nil
	}

	if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}

	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}

	block, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		return nil, err
	}

	privateKeyPEM := pem.EncodeToMemory(block)

	err = os.WriteFile(path, privateKeyPEM, 0600)
	if err != nil {
		return nil, err
	}

	signer, err := ssh.ParsePrivateKey(privateKeyPEM)
	if err != nil {
		return nil, err
	}

	return signer, nil
}

func listenPublic(addr string, sshConn *ssh.ServerConn) {
	ln, err := net.Listen("tcp", addr)

	if err != nil {
		log.Println("listen error:", err)
		return
	}

	defer ln.Close()

	log.Println("public listener started on", addr)

	for {
		conn, err := ln.Accept()

		if err != nil {
			log.Println("accept error:", err)
			continue
		}

		go func(userConn net.Conn) {

			channel, requests, err := sshConn.OpenChannel(
				"tunnel",
				nil,
			)

			if err != nil {
				log.Println("open channel error:", err)
				return
			}

			defer channel.Close()
			go ssh.DiscardRequests(requests)

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				io.Copy(channel, userConn)
			}()

			go func() {
				defer wg.Done()
				io.Copy(userConn, channel)
			}()

			wg.Wait()

		}(conn)
	}

}
