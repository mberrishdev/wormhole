package main

import (
	"bufio"
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"encoding/pem"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"math/big"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"golang.org/x/crypto/ssh"
)

var (
	mu      sync.Mutex
	tunnels = make(map[string]*ssh.ServerConn)
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

	go listenPublic(":443")

	go http.ListenAndServe(":80", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.Host+r.RequestURI, 301)
	}))

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

	// cleanup when client disconnects
	defer func() {
		mu.Lock()
		for token, c := range tunnels {
			if c == sshConn {
				delete(tunnels, token)
				log.Printf("tunnel removed: %s", token)
			}
		}
		mu.Unlock()
	}()

	for req := range reqs {
		log.Printf("received request: type=%s", req.Type)
		if req.Type == "forward" {
			token := randomToken()

			mu.Lock()
			tunnels[token] = sshConn
			mu.Unlock()

			log.Printf("tunnel registered: %s.wormhole.mberrishdev.me", token)
			req.Reply(true, []byte(token))
		}
	}
}

func listenPublic(addr string) {
	cert, err := tls.LoadX509KeyPair(
		"/etc/letsencrypt/live/wormhole.mberrishdev.me/fullchain.pem",
		"/etc/letsencrypt/live/wormhole.mberrishdev.me/privkey.pem",
	)

	if err != nil {
		log.Fatal("load cert:", err)
	}

	log.Println("TLS cert loaded successfully")

	tlsConfig := &tls.Config{Certificates: []tls.Certificate{cert}}

	ln, err := tls.Listen("tcp", addr, tlsConfig)

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
			sshConn, reader, ok := lookupTunnel(userConn)
			if !ok {
				userConn.Close()
				return
			}

			remoteIP, _, _ := net.SplitHostPort(userConn.RemoteAddr().String())
			channel, requests, err := sshConn.OpenChannel("tunnel", []byte(remoteIP))
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

func lookupTunnel(conn net.Conn) (*ssh.ServerConn, io.Reader, bool) {
	br := bufio.NewReader(conn)

	req, err := http.ReadRequest(br)
	if err != nil {
		log.Println("invalid request:", err)
		return nil, nil, false
	}

	host := req.Host
	parts := strings.Split(host, ".")

	if len(parts) == 0 {
		conn.Write([]byte("HTTP/1.1 400 Bad Request\r\n\r\nBad Request"))
		return nil, nil, false
	}

	token := parts[0]

	mu.Lock()
	sshConn, ok := tunnels[token]
	mu.Unlock()

	if !ok {
		conn.Write([]byte("HTTP/1.1 404 Not Found\r\n\r\nTunnel not found"))
		return nil, nil, false
	}

	var replay bytes.Buffer
	req.Write(&replay)

	return sshConn, io.MultiReader(&replay, br), true
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

func randomToken() string {
	adjectives := []string{
		"happy", "quick", "brave", "calm", "bright",
		"swift", "cool", "smart", "silent", "wild",
	}

	animals := []string{
		"panda", "fox", "wolf", "bear", "hawk",
		"lion", "tiger", "eagle", "falcon", "lynx",
	}

	// pick random adjective
	adjIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(adjectives))))
	adj := adjectives[adjIdx.Int64()]

	// pick random animal
	anIdx, _ := rand.Int(rand.Reader, big.NewInt(int64(len(animals))))
	animal := animals[anIdx.Int64()]

	// 64-bit random number for high uniqueness
	var b [8]byte
	_, _ = rand.Read(b[:])

	num := int64(b[0])<<56 |
		int64(b[1])<<48 |
		int64(b[2])<<40 |
		int64(b[3])<<32 |
		int64(b[4])<<24 |
		int64(b[5])<<16 |
		int64(b[6])<<8 |
		int64(b[7])

	if num < 0 {
		num = -num
	}

	num = num % 1000000

	return fmt.Sprintf("%s-%s-%06d", adj, animal, num)
}
