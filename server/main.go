package main

import (
	"crypto/tls"
	"fmt"
	"log"
	"os"
)

func main() {
	// Load server certificate and key
	serverCert, err := os.ReadFile("server.pem")
	if err != nil {
		log.Fatalf("failed to load server key pair: %v", err)
	}

	serverKey, err := os.ReadFile("server-key.pem")
	if err != nil {
		log.Fatalf("failed to load server key pair: %v", err)
	}

	// Load CA certificate to verify client certs
	caCert, err := os.ReadFile("ca.pem")
	if err != nil {
		log.Fatalf("failed to read CA cert: %v", err)
	}

	server, err := NewServer(&ServerConfig{
		Port:          8443,
		Host:          "localhost",
		CaCertPem:     caCert,
		ServerCertPem: serverCert,
		ServerKeyPem:  serverKey,
		ParseIdentity: func(conn *tls.Conn) (string, error) {
			state := conn.ConnectionState()
			if len(state.PeerCertificates) == 0 {
				return "", fmt.Errorf("no client cert")
			}
			clientCert := state.PeerCertificates[0]
			return clientCert.Subject.CommonName, nil
		},
		ProcessRequest: func(request *Request) ([]byte, error) {
			return []byte("Hello, client!"), nil
		},
	})

	if err != nil {
		log.Fatalf("failed to start server: %v", err)
	}

	defer server.Shutdown()

	// Start the server
	er := server.HandleConnections()
	if er != nil {
		log.Fatalf("failed to handle connections: %v", er)
	}
	fmt.Println("Server stopped")

}
