package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/mcp"
	"github.com/mark3labs/mcp-go/server"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

func main() {
	gatewayAddr := os.Getenv("PENFOLD_GATEWAY_ADDR")
	if gatewayAddr == "" {
		gatewayAddr = "localhost:50051"
	}

	listenAddr := os.Getenv("MCP_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":50055"
	}

	transportCreds := grpc.WithTransportCredentials(insecure.NewCredentials())
	if caPath := os.Getenv("PENFOLD_GATEWAY_TLS_CA"); caPath != "" {
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			log.Fatalf("failed to read CA cert: %v", err)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs: pool,
		}))
	}

	conn, err := grpc.NewClient(gatewayAddr, transportCreds)
	if err != nil {
		log.Fatalf("failed to connect to gateway: %v", err)
	}
	defer conn.Close()

	searchClient := searchv1.NewSearchServiceClient(conn)

	mcpServer := server.NewMCPServer(
		"penfold",
		"0.1.0",
		server.WithToolCapabilities(false),
	)

	// Generate JSON Schema from proto descriptor
	schema := schemaFromProto((&searchv1.SearchRequest{}).ProtoReflect().Descriptor())
	schemaJSON, err := json.Marshal(schema)
	if err != nil {
		log.Fatalf("failed to marshal schema: %v", err)
	}

	searchTool := mcp.NewToolWithRawSchema(
		"penfold_search",
		"Search Penfold's knowledge base using keywords and semantic matching. Returns matching emails, meetings, and documents with relevance scores.",
		schemaJSON,
	)
	searchTool.Annotations = mcp.ToolAnnotation{
		ReadOnlyHint: boolPtr(true),
	}

	searchHandler := grpcHandler(
		func() *searchv1.SearchRequest { return new(searchv1.SearchRequest) },
		searchClient.Search,
		DefaultFormatter(),
	)

	mcpServer.AddTool(searchTool, searchHandler)

	httpServer := server.NewStreamableHTTPServer(mcpServer)

	log.Printf("penfold-mcp starting on %s (upstream: %s)", listenAddr, gatewayAddr)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := httpServer.Start(listenAddr); err != nil {
			log.Fatalf("server error: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("shutting down...")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		log.Printf("shutdown error: %v", err)
	}
}

func boolPtr(b bool) *bool { return &b }
