package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
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

	gatewayClient := gatewayv1.NewGatewayServiceClient(conn)
	searchClient := searchv1.NewSearchServiceClient(conn)
	assertionsClient := assertionsv1.NewAssertionsServiceClient(conn)
	contentClient := contentv1.NewContentProcessorServiceClient(conn)
	threadsClient := threadsv1.NewThreadsServiceClient(conn)

	// Define available toolsets. The search toolset is fully wired;
	// remaining toolsets are stubs for future phases.
	toolsets := []*Toolset{
		searchToolset(searchClient, assertionsClient, contentClient, threadsClient),
		{Name: "knowledge", Description: "Access assertions, relationships, and extracted knowledge"},
		{Name: "entities", Description: "Browse and search people, organizations, and other entities"},
		{Name: "content", Description: "Read full email text, threads, and document content"},
		{Name: "ops", Description: "System operations — health checks, stats, diagnostics"},
		{Name: "workflow", Description: "Pipeline monitoring and content processing status"},
	}

	// Build the toolset manager first so we can get its hooks.
	tm := NewToolsetManager(nil, toolsets) // mcpServer set below after hooks are wired
	tm.RegisterHooks()

	mcpServer := server.NewMCPServer(
		"penfold",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithHooks(tm.Hooks()),
	)

	// Wire the server back into the manager now that it exists.
	tm.mcpServer = mcpServer

	// Register global tools.
	tm.RegisterMetaTools()
	registerHealthTool(mcpServer, gatewayClient)

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
