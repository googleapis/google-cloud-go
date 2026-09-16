package main

import (
	"context"
	"fmt"
	"log"
	"sync"

	"cloud.google.com/go/storage"
	"cloud.google.com/go/storage/experimental"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// 1. Custom BufferPool
type SimpleBufferPool struct {
	pool sync.Pool
}

func NewSimpleBufferPool() *SimpleBufferPool {
	return &SimpleBufferPool{
		pool: sync.Pool{
			New: func() any {
				b := make([]byte, 8*1024*1024)
				return &b
			},
		},
	}
}

func (p *SimpleBufferPool) Get(maxSize int) ([]byte, error) {
	bufPtr := p.pool.Get().(*[]byte)
	buf := *bufPtr
	if len(buf) > maxSize {
		buf = buf[:maxSize]
	}
	return buf, nil
}

func (p *SimpleBufferPool) TryGet(maxSize int) ([]byte, error) {
	return p.Get(maxSize)
}

func (p *SimpleBufferPool) Put(buf []byte) {
	p.pool.Put(&buf)
}

func main() {
	ctx := context.Background()

	fmt.Println("--- Starting Parallel Upload Flow Demo ---")

	// STEP 1: Buffer pool banaya
	myPool := NewSimpleBufferPool()
	fmt.Println("[STEP 1] Buffer pool successfully created.")

	// STEP 2: Client banaya experimental options ke sath
	// (Mock endpoint aur WithoutAuthentication use kiya taki bina GCP credentials ke bhi run ho sake)
	client, err := storage.NewGRPCClient(ctx,
		option.WithoutAuthentication(),
		option.WithEndpoint("localhost:9999"),
		option.WithGRPCDialOption(grpc.WithTransportCredentials(insecure.NewCredentials())),

		experimental.WithBufferPool(myPool),
	)
	if err != nil {
		log.Fatalf("Client banate waqt error aaya: %v", err)
	}
	defer client.Close()
	fmt.Println("[STEP 2] Client created successfully with WithBufferPool!")

	// STEP 3: Writer banaya
	bucket := client.Bucket("demo-bucket")
	obj := bucket.Object("sample-data.bin")
	w := obj.NewWriter(ctx)
	fmt.Println("[STEP 3] Writer created successfully.")

	// STEP 4: Abort call kiya
	fmt.Println("[STEP 4] Calling w.Abort()...")
	err = w.Abort()
	println("Error : ", err.Error())

	// STEP 5: BufferPool Get & Put verify kiya
	buf, err := myPool.Get(1024)
	if err != nil {
		log.Fatalf("Pool.Get error: %v", err)
	}
	fmt.Printf("[STEP 5] Buffer allocated from pool: len=%d bytes\n", len(buf))
	myPool.Put(buf)
	fmt.Println("[STEP 5] Buffer returned to pool successfully.")

	fmt.Println("--- Demo completed successfully! ---")
}
