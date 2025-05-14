package main

import (
	"bufio"
	"fmt"
	"net"
	"sync"
	"time"
)

var (
	activeConnections int
	totalConnections  int
	failedConnections int
	mu                sync.Mutex
)

func incrementActive() {
	mu.Lock()
	activeConnections++
	totalConnections++
	mu.Unlock()
}

func decrementActive() {
	mu.Lock()
	activeConnections--
	mu.Unlock()
}

func incrementFailures() {
	mu.Lock()
	failedConnections++
	mu.Unlock()
}

func statsSnapshot() (int, int, int) {
	mu.Lock()
	defer mu.Unlock()
	return activeConnections, totalConnections, failedConnections
}

func maintainConnection(id int) {
	conn, err := net.Dial("tcp", "65.2.189.137:9000")
	if err != nil {
		incrementFailures()
		return
	}
	incrementActive()
	defer func() {
		decrementActive()
		conn.Close()
	}()

	reader := bufio.NewReader(conn)

	for {
		_, err := conn.Write([]byte("ping\n"))
		if err != nil {
			incrementFailures()
			return
		}

		_, err = reader.ReadString('\n')
		if err != nil {
			incrementFailures()
			return
		}

		time.Sleep(100 * time.Millisecond)
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func printStats() {
	for {
		time.Sleep(1 * time.Second)
		clearScreen()
		active, total, failed := statsSnapshot()
		fmt.Println("TCP Client Load Tester")
		fmt.Println("----------------------")
		fmt.Printf("Active connections   : %d\n", active)
		fmt.Printf("Total connections    : %d\n", total)
		fmt.Printf("Failed connections   : %d\n", failed)
	}
}

func main() {
	go printStats()

	id := 0
	for {
		go maintainConnection(id)
		id++
		time.Sleep(10 * time.Millisecond)
	}
}

