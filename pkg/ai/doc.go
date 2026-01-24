// Package ai provides a gRPC client for the AI Coordinator service.
//
// The Client type implements the AIClient interface from services/worker/activities,
// providing access to AI capabilities including embedding generation, summarization,
// and assertion extraction.
//
// # Basic Usage
//
//	client, err := ai.NewClient("localhost:50051")
//	if err != nil {
//		log.Fatal(err)
//	}
//	defer client.Close()
//
//	// Generate an embedding
//	resp, err := client.GenerateEmbedding(ctx, &aiv1.EmbeddingRequest{
//		Text: "Hello, world!",
//	})
//
// # Configuration
//
// The client supports various configuration options:
//
//	client, err := ai.NewClient("localhost:50051",
//		ai.WithRequestTimeout(60*time.Second),
//		ai.WithMaxRetries(5),
//		ai.WithRetryBackoff(200*time.Millisecond),
//	)
//
// # Retry Behavior
//
// By default, the client retries failed requests up to 3 times with exponential
// backoff. Only transient errors (Unavailable, DeadlineExceeded, ResourceExhausted,
// Aborted, Internal) are retried. Permanent errors (InvalidArgument, NotFound, etc.)
// are returned immediately.
//
// # Health Checks
//
// Use the HealthCheck method to verify the connection to the AI service:
//
//	if err := client.HealthCheck(ctx); err != nil {
//		log.Printf("AI service unhealthy: %v", err)
//	}
package ai
