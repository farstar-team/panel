package service

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"
	"testing"

	"golang.org/x/crypto/ssh"
)

func TestSSHRunDoesNotLoseConcurrentStandardStreams(t *testing.T) {
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{NoClientAuth: true}
	config.AddHostKey(signer)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		server, channels, requests, err := ssh.NewServerConn(conn, config)
		if err != nil {
			return
		}
		defer server.Close()
		go ssh.DiscardRequests(requests)
		channelRequest, ok := <-channels
		if !ok {
			return
		}
		channel, requests, err := channelRequest.Accept()
		if err != nil {
			return
		}
		defer channel.Close()
		request, ok := <-requests
		if !ok {
			return
		}
		_ = request.Reply(true, nil)
		var writers sync.WaitGroup
		for index, stream := range []io.Writer{channel, channel.Stderr()} {
			writers.Add(1)
			go func() {
				defer writers.Done()
				chunk := bytes.Repeat([]byte{byte('a' + index)}, 4096)
				for range 100 {
					_, _ = stream.Write(chunk)
				}
			}()
		}
		writers.Wait()
		var status [4]byte
		binary.BigEndian.PutUint32(status[:], 0)
		_, _ = channel.SendRequest("exit-status", false, status[:])
		_ = channel.Close()
		_ = server.Wait()
	}()
	client, err := ssh.Dial("tcp", listener.Addr().String(), &ssh.ClientConfig{User: "test", HostKeyCallback: ssh.FixedHostKey(signer.PublicKey())})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	output, err := sshRun(client, "emit-streams", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(output) != 819200 || bytes.Count(output, []byte("a")) != 409600 || bytes.Count(output, []byte("b")) != 409600 {
		t.Fatalf("concurrent output lost: bytes=%d a=%d b=%d", len(output), bytes.Count(output, []byte("a")), bytes.Count(output, []byte("b")))
	}
	_ = client.Close()
	<-serverDone
}
