package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

type Server struct {
	ctx      context.Context
	cancelFn context.CancelFunc
	config   *ServerConfig
	listener net.Listener
}

type RequestHandler func(request *Request) ([]byte, error)

type ServerConfig struct {
	Port           int
	Host           string
	CaCertPem      []byte
	ServerCertPem  []byte
	ServerKeyPem   []byte
	ParseIdentity  func(conn *tls.Conn) (string, error)
	ProcessRequest RequestHandler
}

type RequestType uint8

type Request struct {
	Length uint16
	Id     [4]byte
	Type   RequestType
	Body   []byte
}

type Response struct {
	Length uint16  // 2 bytes
	Id     [4]byte // 4 bytes
	Body   []byte
}

type Connection struct {
	Conn            *tls.Conn
	Identity        string
	CurRequest      *Request
	RequestChannel  chan *Request
	ResponseChannel chan *Response
	ctx             context.Context
	cancel          context.CancelFunc
	resetCh         chan struct{}
}

func NewServer(config *ServerConfig) (*Server, error) {
	// Load server certificate and key
	serverCert, err := tls.X509KeyPair(config.ServerCertPem, config.ServerKeyPem)
	if err != nil {
		return nil, err
	}

	// Load CA certificate to verify client certs
	caPool := x509.NewCertPool()
	if ok := caPool.AppendCertsFromPEM(config.CaCertPem); !ok {
		return nil, fmt.Errorf("failed to load CA cert")
	}

	// Create listener
	listener, err := tls.Listen("tcp", fmt.Sprintf("%s:%d", config.Host, config.Port), &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientAuth:   tls.RequireAndVerifyClientCert, // Require client cert
		ClientCAs:    caPool,
		MinVersion:   tls.VersionTLS13,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to listen on %s:%d: %v", config.Host, config.Port, err)
	}

	// Create context with cancel function
	ctx, cancelFn := context.WithCancel(context.Background())

	return &Server{
		config:   config,
		listener: listener,
		ctx:      ctx,
		cancelFn: cancelFn,
	}, nil
}

func (s *Server) HandleConnections() error {
	if s.listener == nil {
		return fmt.Errorf("listener is not initialized")
	}
	defer s.listener.Close()

	for {
		// Check if we should stop accepting connections
		select {
		case <-s.ctx.Done():
			return nil
		default:
		}

		conn, err := s.listener.Accept()
		if err != nil {
			netErr, ok := err.(net.Error)
			if ok && netErr.Timeout() {
				// This is just a timeout to check context, continue
				continue
			}

			if s.ctx.Err() != nil {
				// Server is shutting down
				return nil
			}

			fmt.Printf("Error accepting connection: %v\n", err)
			continue
		}

		tlsConn, ok := conn.(*tls.Conn)
		if !ok {
			conn.Close()
			fmt.Println("Connection is not TLS, rejecting")
			continue
		}

		fmt.Println("Accepted new connection")

		// Force handshake with timeout
		err = tlsConn.SetDeadline(time.Now().Add(5 * time.Second))
		if err == nil {
			err = tlsConn.Handshake()
		}
		if err != nil {
			tlsConn.Close()
			fmt.Printf("TLS handshake failed: %v\n", err)
			continue
		}

		fmt.Println("TLS handshake completed")

		// Reset deadline after handshake
		tlsConn.SetDeadline(time.Time{})

		// Parse identity from connection
		identity, err := s.config.ParseIdentity(tlsConn)
		if err != nil {
			conn.Close()
			continue
		}

		fmt.Printf("Parsed identity: %s\n", identity)

		// Create connection context as child of server context
		connCtx, connCancel := context.WithCancel(s.ctx)

		connection := &Connection{
			Conn:            tlsConn,
			Identity:        identity,
			RequestChannel:  make(chan *Request, 100),
			ResponseChannel: make(chan *Response, 100),
			ctx:             connCtx,
			cancel:          connCancel,
			resetCh:         make(chan struct{}),
		}

		go s.handleConnection(connection)
	}
}

func (s *Server) handleConnection(conn *Connection) {
	defer close(conn.resetCh)
	defer func() {
		// Close the connection and clean up
		conn.Close()
	}()

	/*
	* We need three goroutines -
	* 1. One goroutine to read packets and parse them into Request objects.
	* 2. One goroutine to read from the RequestChannel and process requests and put responses in the ResponseChannel.
	* 3. One goroutine to read from the ResponseChannel and send responses back to the client.
	 */

	go conn.readPackets()
	go conn.sendResponses()
	go conn.processRequests(s.config.ProcessRequest)

	<-conn.ctx.Done()
}

func (c *Connection) readPackets() {
	defer func() {
		fmt.Println("readPackets goroutine exiting")
	}()
	// Read packets and parse them into Request objects
	reader := bufio.NewReader(c.Conn)
	for {
		// Check if we should stop
		select {
		case <-c.ctx.Done():
			fmt.Println("Context done, stopping readPackets")
			return
		case <-c.resetCh:
			fmt.Println("Reset channel closed, stopping readPackets")
			return
		default:
		}

		// Read the length of the request (2 bytes)
		lengthBytes := make([]byte, 2)
		_, err := io.ReadFull(reader, lengthBytes)
		if err != nil {
			c.cancel() // Signal other goroutines to stop
			return
		}
		totalLength := binary.BigEndian.Uint16(lengthBytes)
		if totalLength < 7 /* 2 + 4 + 1*/ {
			fmt.Println("Invalid request length:", totalLength)
			c.cancel() // Signal other goroutines to stop
			return
		}

		// Read the rest of the request
		requestBytes := make([]byte, totalLength-2)
		_, err = io.ReadFull(reader, requestBytes)
		if err != nil {
			c.cancel() // Signal other goroutines to stop
			return
		}
		request := &Request{
			Length: totalLength,
			Id:     [4]byte{requestBytes[0], requestBytes[1], requestBytes[2], requestBytes[3]},
			Type:   RequestType(requestBytes[4]),
			Body:   requestBytes[5:],
		}

		// Send the request to the request channel with context awareness
		select {
		case c.RequestChannel <- request:
		case <-c.ctx.Done():
			return
		case <-c.resetCh:
			return
		}
	}
}

func (c *Connection) processRequests(processRequest RequestHandler) {
	defer func() {
		fmt.Println("processRequests goroutine exiting")
	}()
	// Process requests from the RequestChannel
	for {
		select {
		case request, ok := <-c.RequestChannel:
			if !ok {
				// Channel closed
				return
			}

			fmt.Println("Processing request:", request.Id)
			body, err := processRequest(request)
			if err != nil {
				fmt.Println("Error processing request:", err)
				continue
			}

			// Prepare the response
			response := &Response{
				Length: uint16(len(body) + 7), // 2 bytes for length, 4 bytes for ID, 1 byte for type
				Id:     request.Id,
				Body:   body,
			}

			// Try to send the response, respecting context
			select {
			case c.ResponseChannel <- response:
				// Response sent to channel
			case <-c.ctx.Done():
				return
			case <-c.resetCh:
				return
			}
		case <-c.ctx.Done():
			return
		case <-c.resetCh:
			return
		}
	}
}

func (c *Connection) sendResponses() {
	defer func() {
		fmt.Println("sendResponses goroutine exiting")
	}()
	// Send responses from the ResponseChannel to the client
	writer := bufio.NewWriter(c.Conn)
	for {
		select {
		case response, ok := <-c.ResponseChannel:
			if !ok {
				// Channel closed
				return
			}
			responseBytes := make([]byte, response.Length)
			binary.BigEndian.PutUint16(responseBytes[:2], response.Length)
			copy(responseBytes[2:6], response.Id[:])
			copy(responseBytes[6:], response.Body)

			_, err := writer.Write(responseBytes)
			if err != nil {
				fmt.Println("Error writing response:", err)
				c.cancel() // Signal other goroutines to stop
				return
			}
			err = writer.Flush()
			if err != nil {
				fmt.Println("Error flushing writer:", err)
				c.cancel() // Signal other goroutines to stop
				return
			}
		case <-c.ctx.Done():
			return
		case <-c.resetCh:
			return
		}
	}
}

func (c *Connection) Close() {
	if c.Conn != nil {
		c.Conn.Close()
	}

	// Signal cancellation
	c.cancel()

	// Close channels
	close(c.RequestChannel)
	close(c.ResponseChannel)
}

// Shutdown gracefully shuts down the server
func (s *Server) Shutdown() error {
	// Signal server shutdown
	s.cancelFn()

	// Close the listener to stop accepting new connections
	return s.listener.Close()
}
