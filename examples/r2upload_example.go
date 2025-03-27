package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/boloc/go-frame-server/pkg/frame/client"
)

func main() {
	// Create R2 client configuration
	r2Config := &client.R2Config{
		AccountID:       "your-cloudflare-account-id",
		AccessKeyID:     "your-r2-access-key-id",
		AccessKeySecret: "your-r2-access-key-secret",
		BucketName:      "your-bucket-name",
		Region:          "auto", // Usually "auto" for R2
		Endpoint:        "https://your-r2-endpoint",
		CustomDomain:    "https://static.your-domain.com",
	}

	// Create new R2 client
	r2Client, err := client.NewR2Client(r2Config)
	if err != nil {
		log.Fatalf("Failed to create R2 client: %v", err)
	}

	// Example 1: Upload a file from disk
	ctx := context.Background()
	uploadResult, err := r2Client.UploadFile(ctx, "./example.jpg", "uploads/example.jpg", "image/jpeg")
	if err != nil {
		log.Fatalf("Failed to upload file: %v", err)
	}
	fmt.Printf("File uploaded successfully!\n")
	fmt.Printf("URL: %s\n", uploadResult.URL)
	fmt.Printf("Key: %s\n", uploadResult.Key)
	fmt.Printf("Size: %d bytes\n", uploadResult.Size)
	fmt.Printf("Content Type: %s\n", uploadResult.ContentType)

	// Example 2: Upload bytes
	data := []byte("Hello, R2!")
	bytesResult, err := r2Client.UploadBytes(ctx, data, "text/hello.txt", "text/plain")
	if err != nil {
		log.Fatalf("Failed to upload bytes: %v", err)
	}
	fmt.Printf("Bytes uploaded successfully!\n")
	fmt.Printf("URL: %s\n", bytesResult.URL)

	// Example 3: Generate presigned URL (temporary access URL)
	presignedURL, err := r2Client.GeneratePresignedURL(ctx, uploadResult.Key, 1*time.Hour)
	if err != nil {
		log.Fatalf("Failed to generate presigned URL: %v", err)
	}
	fmt.Printf("Presigned URL (valid for 1 hour): %s\n", presignedURL)

	// Example 4: List objects with prefix
	objects, err := r2Client.ListObjects(ctx, "uploads/", 10)
	if err != nil {
		log.Fatalf("Failed to list objects: %v", err)
	}
	fmt.Printf("Objects in uploads/ prefix:\n")
	for _, key := range objects {
		fmt.Printf("- %s\n", key)
	}

	// Example 5: Delete an object
	err = r2Client.DeleteObject(ctx, uploadResult.Key)
	if err != nil {
		log.Fatalf("Failed to delete object: %v", err)
	}
	fmt.Printf("Object deleted successfully: %s\n", uploadResult.Key)
}
