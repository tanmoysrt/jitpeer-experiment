package main

// import (
// 	"bufio"
// 	"crypto/tls"
// 	"crypto/x509"
// 	"fmt"
// 	"log"
// 	"os"
// 	"sync"
// )

// func main() {
	// // Load client certificate and key
	// clientCert, err := tls.LoadX509KeyPair("client.pem", "client-key.pem")
	// if err != nil {
	// 	log.Fatalf("failed to load client key pair: %v", err)
	// }

	// // Load CA certificate to verify server
	// caCert, err := os.ReadFile("ca.pem")
	// if err != nil {
	// 	log.Fatalf("failed to read CA cert: %v", err)
	// }
	// caPool := x509.NewCertPool()
	// if ok := caPool.AppendCertsFromPEM(caCert); !ok {
	// 	log.Fatal("failed to append CA cert")
	// }

	// // TLS config
	// tlsCfg := &tls.Config{
	// 	Certificates:       []tls.Certificate{clientCert},
	// 	RootCAs:            caPool,
	// 	MinVersion:         tls.VersionTLS13,
	// 	ServerName:         "server.local",
	// 	InsecureSkipVerify: false,
	// 	ClientSessionCache: tls.NewLRUClientSessionCache(10),
	// }

// 	wg := new(sync.WaitGroup)
// 	// Spawn up 100s of goroutines to call the server
// 	for i := 0; i < 10000; i++ {
// 		wg.Add(1)
// 		go callServer(tlsCfg, wg)
// 	}
// 	wg.Wait()
// }

// func callServer(tlsCfg *tls.Config, wg *sync.WaitGroup) {
	// // Dial TLS connection to server
	// conn, err := tls.Dial("tcp", "localhost:8443", tlsCfg)
	// if err != nil {
	// 	log.Fatalf("failed to connect: %v", err)
	// }
	// defer func() {
	// 	conn.Close()
	// 	wg.Done()
	// }()

// 	log.Println("Connected to server")

// 	// Send a line
// 	writer := bufio.NewWriter(conn)
// 	message := "PING\n"
// 	_, err = writer.WriteString(message)
// 	if err != nil {
// 		log.Fatalf("failed to write: %v", err)
// 	}
// 	writer.Flush()
// 	log.Printf("Sent: %s", message)

// 	// Read server response
// 	reader := bufio.NewReader(conn)
// 	resp, err := reader.ReadString('\n')
// 	if err != nil {
// 		log.Fatalf("failed to read response: %v", err)
// 	}
// 	fmt.Printf("Server replied: %s", resp)
// }
