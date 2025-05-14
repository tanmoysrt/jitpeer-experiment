package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"runtime/pprof"
	"sync/atomic"
	"time"

	"github.com/dgraph-io/badger/v4"
	_ "net/http/pprof"
)

func randomDomain() string {
	return fmt.Sprintf("%x.com", rand.Int63())
}

func randomIP() string {
	return fmt.Sprintf("%d.%d.%d.%d", rand.Intn(256), rand.Intn(256), rand.Intn(256), rand.Intn(256))
}

func main() {
	rand.Seed(time.Now().UnixNano())

	// Start pprof server
	go func() {
		log.Println("pprof listening at :6060")
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()

	// Start CPU profiling
	cpuFile, err := os.Create("cpu.prof")
	if err != nil {
		log.Fatal(err)
	}
	pprof.StartCPUProfile(cpuFile)
	defer pprof.StopCPUProfile()

	// Open BadgerDB
	db, err := badger.Open(badger.DefaultOptions("dns.db").WithLogger(nil))
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	// Insert 10,000 random records
	keys := make([]string, 10000)
	log.Println("Inserting 10k records...")
	for i := 0; i < 10000; i++ {
		domain := randomDomain()
		ip := randomIP()
		keys[i] = domain

		err := db.Update(func(txn *badger.Txn) error {
			return txn.Set([]byte(domain), []byte(ip))
		})
		if err != nil {
			log.Fatal(err)
		}
	}
	log.Println("Insertion complete.")

	// Run 500 concurrent Get() calls
	log.Println("Starting 500 goroutines for Get calls...")
	var successCount int32
	start := time.Now()

	done := make(chan struct{}, 500)
	for i := 0; i < 500; i++ {
		go func() {
			key := []byte(keys[rand.Intn(len(keys))])
			err := db.View(func(txn *badger.Txn) error {
				_, err := txn.Get(key)
				if err == nil {
					atomic.AddInt32(&successCount, 1)
				}
				return nil
			})
			if err != nil {
				log.Println("View txn error:", err)
			}
			done <- struct{}{}
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 500; i++ {
		<-done
	}
	log.Printf("All reads completed in %s", time.Since(start))
	log.Printf("Successful reads: %d / 500", successCount)

	// Count total number of keys
	var itemCount int
	err = db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Rewind(); it.Valid(); it.Next() {
			itemCount++
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("Total keys in DB: %d", itemCount)

	// Write memory profile
	runtime.GC()
	memFile, err := os.Create("mem.prof")
	if err != nil {
		log.Fatal(err)
	}
	defer memFile.Close()
	pprof.WriteHeapProfile(memFile)

	log.Println("CPU and memory profiles written to 'cpu.prof' and 'mem.prof'")
}

