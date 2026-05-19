package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

func main() {
	var sshPort int
	flag.IntVar(&sshPort, "ssh-port", 2222, "SSH listen port")
	flag.Parse()

	signer, err := loadOrGenerateKey("server.key")
	if err != nil {
		log.Fatal(err)
	}

	config := &ssh.ServerConfig{
		PasswordCallback: func(conn ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
			if string(password) == "secret123" {
				return nil, nil
			}
			return nil, fmt.Errorf("rejected")
		},
	}
	config.AddHostKey(signer)

	addr := fmt.Sprintf(":%d", sshPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer ln.Close()

	log.Println("SSH server listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Println("accept error:", err)
			continue
		}
		go handleClient(conn, config)
	}
}

func handleClient(conn net.Conn, config *ssh.ServerConfig) {
	sshConn, _, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		log.Println("handshake failed:", err)
		conn.Close()
		return
	}
	defer sshConn.Close()

	log.Println("tunnel client connected:", sshConn.RemoteAddr())

	for req := range reqs {
		if req.Type == "forward" {
			parts := strings.SplitN(string(req.Payload), ":", 2)
			if len(parts) != 2 {
				req.Reply(false, nil)
				continue
			}

			port := parts[0]
			token := parts[1]

			log.Println("client wants to forward port:", port, "with token")

			go listenPublic(":"+port, sshConn, token)
			req.Reply(true, nil)
		}
	}
}

func listenPublic(addr string, sshConn *ssh.ServerConn, token string) {
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
			reader, ok := checkToken(userConn, token)
			if !ok {
				userConn.Close()
				return
			}

			channel, requests, err := sshConn.OpenChannel("tunnel", nil)
			if err != nil {
				log.Println("open channel error:", err)
				userConn.Close()
				return
			}
			defer channel.Close()
			go ssh.DiscardRequests(requests)

			var wg sync.WaitGroup
			wg.Add(2)

			go func() {
				defer wg.Done()
				io.Copy(channel, reader)
			}()

			go func() {
				defer wg.Done()
				io.Copy(userConn, channel)
			}()

			wg.Wait()
		}(conn)
	}
}

func checkToken(conn net.Conn, secret string) (io.Reader, bool) {
	br := bufio.NewReader(conn)

	req, err := http.ReadRequest(br)
	if err != nil {
		return nil, false
	}

	token := req.URL.Query().Get("token")
	if token != secret {
		conn.Write([]byte("HTTP/1.1 403 Forbidden\r\n\r\nForbidden"))
		return nil, false
	}

	var replay bytes.Buffer
	err = req.Write(&replay)
	if err != nil {
		return nil, false
	}

	return io.MultiReader(&replay, br), true
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
