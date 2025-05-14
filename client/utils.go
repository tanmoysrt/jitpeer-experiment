package main

import (
	"sync/atomic"
	"time"
)

// Atomic counter for the 24-bit counter (using a 32-bit integer but masking the upper 8 bits)
var counter atomic.Uint32

func init() {
	counter.Store(0) // Initialize the counter to 0
}

// Function to get the last byte of the nano timestamp (8 bits)
func getNanoTimestampByte() byte {
	nano := uint32(time.Now().UnixNano())
	return byte(nano)
}

// Function to get the next 24-bit counter value (atomic, wraps at 16,777,216)
func getNextCounter() [3]byte {
	// Atomically increment the counter, using only the lower 24 bits
	for {
		current := counter.Load()
		next := (current + 1) & ((1 << 24) - 1) // Wrap the counter at 24 bits

		// Use CompareAndSwap to ensure atomicity
		if counter.CompareAndSwap(current, next) {
			// Convert the 24-bit counter into its byte representation (3 bytes)
			var counterBytes [3]byte
			counterBytes[0] = byte(next >> 16)  // Upper 8 bits of the 24-bit counter
			counterBytes[1] = byte(next >> 8)   // Middle 8 bits of the 24-bit counter
			counterBytes[2] = byte(next & 0xFF) // Lower 8 bits of the 24-bit counter
			return counterBytes
		}
	}
}

// Combine the last byte of nano timestamp and 24-bit counter to create a random state
func GenerateRequestID() [4]byte {
	nanoByte := getNanoTimestampByte()
	counterBytes := getNextCounter()

	// Combine nano byte and counter into a 4-byte random state
	var state [4]byte
	state[0] = nanoByte
	state[1] = counterBytes[0]
	state[2] = counterBytes[1]
	state[3] = counterBytes[2]

	return state
}

// func main() {
// 	// Measure total time and per generation time
// 	startTime := time.Now()

// 	// Generate 100 lakh random states
// 	var states [10000000][4]byte
// 	for i := 0; i < 10000000; i++ {
// 		states[i] = generateRandomState()
// 	}

// 	// Calculate total time and per generation time in nanoseconds
// 	totalDuration := time.Since(startTime)
// 	perGenerationTime := totalDuration.Nanoseconds() / 10000000

// 	// Output the results
// 	fmt.Printf("Total time: %v\n", totalDuration)
// 	fmt.Printf("Per generation time: %d ns\n", perGenerationTime)
// }
