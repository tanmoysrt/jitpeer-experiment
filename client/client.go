package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

type Client struct {
	Conn           *tls.Conn
	RequestQueue   chan *Request
	ResponseQueue  chan *Response
	PendingMap     map[[4]byte]chan []byte
	PendingM       sync.Mutex
	ShutdownCtx    context.Context
	ShutdownCancel context.CancelFunc
}

type Request struct {
	ID      [4]byte
	ReqType uint8
	Body    []byte
	RespCh  chan []byte
	ErrCh   chan error
}

type Response struct {
	ID   [4]byte
	Body []byte
}

func NewClient(conn *tls.Conn) *Client {
	shutdownCtx, shutdownCancel := context.WithCancel(context.Background())

	client := &Client{
		Conn:           conn,
		RequestQueue:   make(chan *Request, 100),
		ResponseQueue:  make(chan *Response, 100),
		PendingMap:     make(map[[4]byte]chan []byte),
		ShutdownCtx:    shutdownCtx,
		ShutdownCancel: shutdownCancel,
	}

	go client.sendLoop()
	go client.receiveLoop()

	return client
}

func (c *Client) sendLoop() {
	for req := range c.RequestQueue {
		// Send the request over the connection
		err := c.sendRequest(req)
		if err != nil {
			fmt.Printf("Failed to send request (id: %x, type: %d): %v\n", req.ID, req.ReqType, err)
			req.ErrCh <- err
			continue
		}

		fmt.Printf("Request sent (id: %x, type: %d)\n", req.ID, req.ReqType)

		// Wait for the response or timeout
		select {
		case resp := <-c.ResponseQueue:
			if resp.ID == req.ID {
				req.RespCh <- resp.Body
			}
		// case <-time.After(3 * time.Second):
		// 	req.ErrCh <- fmt.Errorf("request timed out (id: %x)", req.ID)
		// }
		default:
		}
	}
}

func (c *Client) sendRequest(req *Request) error {
	// Calculate the total length of the request
	totalLength := 2 + 4 + 1 + len(req.Body)

	// Create the request byte array
	reqBytes := make([]byte, totalLength)

	// Fill in the total length (2 bytes)
	reqBytes[0] = byte(totalLength >> 8) // High byte
	reqBytes[1] = byte(totalLength)      // Low byte

	// Fill in the request ID (4 bytes)
	copy(reqBytes[2:6], req.ID[:])

	// Fill in the request type (1 byte)
	reqBytes[6] = req.ReqType

	// Fill in the body
	copy(reqBytes[7:], req.Body)

	// Write the request to the connection
	_, err := c.Conn.Write(reqBytes)
	if err != nil {
		return fmt.Errorf("failed to send request: %v", err)
	}

	// Store the request in the pending map for tracking responses
	c.PendingM.Lock()
	c.PendingMap[req.ID] = req.RespCh
	c.PendingM.Unlock()

	return nil
}

func (c *Client) receiveLoop() {
	for {
		// Read the first 2 bytes to get the total length
		lengthBuf := make([]byte, 2)
		_, err := c.Conn.Read(lengthBuf)
		if err != nil {
			if err != net.ErrClosed {
				fmt.Println("Error reading length from connection:", err)
			}
			return
		}

		// Calculate the total length
		totalLength := int(lengthBuf[0])<<8 | int(lengthBuf[1])

		// Read the remaining part of the response
		buffer := make([]byte, totalLength-2)
		_, err = c.Conn.Read(buffer)
		if err != nil {
			if err != net.ErrClosed {
				fmt.Println("Error reading body from connection:", err)
			}
			return
		}

		// Extract the response ID (4 bytes)
		respID := [4]byte{}
		copy(respID[:], buffer[:4])

		// Extract the body
		body := buffer[4:]

		// Send the response to the appropriate channel
		c.PendingM.Lock()
		if respCh, exists := c.PendingMap[respID]; exists {
			respCh <- body
			delete(c.PendingMap, respID)
		} else {
			fmt.Printf("Unexpected response ID: %x\n", respID)
		}
		c.PendingM.Unlock()
	}
}

func (c *Client) Send(reqType uint8, body []byte, timeout int) ([]byte, error) {
	// Generate a unique 4-byte ID for this request
	id := GenerateRequestID()

	// Channels to receive response or error
	respCh := make(chan []byte, 1)
	errCh := make(chan error, 1)

	// Enqueue the request for the sendLoop
	c.RequestQueue <- &Request{
		ID:      id,
		ReqType: reqType,
		Body:    body,
		RespCh:  respCh,
		ErrCh:   errCh,
	}

	// Wait for a response, error, or timeout
	select {
	case resp := <-respCh:
		return resp, nil
	case err := <-errCh:
		return nil, err
	case <-time.After(time.Duration(timeout) * time.Millisecond):
		// Timed out: remove from pending map to avoid memory leak
		c.clearPending(id)
		return nil, fmt.Errorf("request timed out (id: %x)", id)
	case <-c.ShutdownCtx.Done():
		return nil, errors.New("client is shutting down")
	}
}

func (c *Client) clearPending(id [4]byte) {
	c.PendingM.Lock()
	delete(c.PendingMap, id)
	c.PendingM.Unlock()
}

func (c *Client) Close() error {
	c.ShutdownCancel()
	return c.Conn.Close()
}

func main() {
	// Load client certificate and key
	clientCert, err := tls.LoadX509KeyPair("client.pem", "client-key.pem")
	if err != nil {
		log.Fatalf("failed to load client key pair: %v", err)
	}

	// Load CA certificate to verify server
	caCert, err := os.ReadFile("ca.pem")
	if err != nil {
		log.Fatalf("failed to read CA cert: %v", err)
	}
	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(caCert); !ok {
		log.Fatal("failed to append CA cert")
	}

	// TLS config
	tlsCfg := &tls.Config{
		Certificates:       []tls.Certificate{clientCert},
		RootCAs:            caPool,
		MinVersion:         tls.VersionTLS13,
		ServerName:         "server.local",
		InsecureSkipVerify: false,
		ClientSessionCache: tls.NewLRUClientSessionCache(10),
	}

	// Dial TLS connection to server
	conn, err := tls.Dial("tcp", "127.0.0.1:8443", tlsCfg)
	if err != nil {
		log.Fatalf("failed to connect: %v", err)
	}
	defer func() {
		conn.Close()
	}()

	client := NewClient(conn)
	defer client.Close()

	wg := new(sync.WaitGroup)
	successResponses := atomic.Int32{}
	// Send a request
	for i := 0; i < 10000; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := client.Send(1, []byte("Hello, Server"), 10000)
			if err != nil {
				fmt.Println("Error sending request:", err)
			} else {
				successResponses.Add(1)

			}
			fmt.Println("Response received:", string(resp))
		}()
	}

	wg.Wait()
	fmt.Printf("Total successful responses: %d\n", successResponses.Load())

	// Print the response
}
