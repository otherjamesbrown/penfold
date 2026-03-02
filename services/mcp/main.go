package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/mark3labs/mcp-go/server"
	assertionsv1 "github.com/otherjamesbrown/penfold/api/proto/assertions/v1"
	contentv1 "github.com/otherjamesbrown/penfold/api/proto/content/v1"
	conversationv1 "github.com/otherjamesbrown/penfold/api/proto/conversation/v1"
	entityv1 "github.com/otherjamesbrown/penfold/api/proto/entity/v1"
	gatewayv1 "github.com/otherjamesbrown/penfold/api/proto/gateway/v1"
	ingestv1 "github.com/otherjamesbrown/penfold/api/proto/ingest/v1"
	aiv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/aipb"
	glossaryv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/glossarypb"
	relationshipv1 "github.com/otherjamesbrown/penfold/api/proto/intelligence/v1/relationshippb"
	pipelinev1 "github.com/otherjamesbrown/penfold/api/proto/pipeline/v1"
	reviewv1 "github.com/otherjamesbrown/penfold/api/proto/processing/v1/reviewpb"
	projectv1 "github.com/otherjamesbrown/penfold/api/proto/project/v1"
	qualityv1 "github.com/otherjamesbrown/penfold/api/proto/quality/v1"
	searchv1 "github.com/otherjamesbrown/penfold/api/proto/search/v1"
	threadsv1 "github.com/otherjamesbrown/penfold/api/proto/threads/v1"
	topicv1 "github.com/otherjamesbrown/penfold/api/proto/topic/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
)

const serverInstructions = `Penfold is James's institutional knowledge management system. Search for Penfold tools when the user asks about: finding information in emails or meetings, searching the knowledge base, looking up people/products/projects, getting briefings or summaries, reviewing new content, checking system health, ingesting new content, or managing glossary terms and topics.`

func main() {
	initLogging()

	gatewayAddr := os.Getenv("PENFOLD_GATEWAY_ADDR")
	if gatewayAddr == "" {
		gatewayAddr = "localhost:50051"
	}
	listenAddr := os.Getenv("MCP_LISTEN_ADDR")
	if listenAddr == "" {
		listenAddr = ":50056"
	}

	transportCreds := grpc.WithTransportCredentials(insecure.NewCredentials())
	if caPath := os.Getenv("PENFOLD_GATEWAY_TLS_CA"); caPath != "" {
		caCert, err := os.ReadFile(caPath)
		if err != nil {
			slog.Error("failed to read CA cert", "error", err)
			os.Exit(1)
		}
		pool := x509.NewCertPool()
		pool.AppendCertsFromPEM(caCert)
		transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{
			RootCAs: pool,
		}))
	}

	conn, err := grpc.NewClient(gatewayAddr, transportCreds)
	if err != nil {
		slog.Error("failed to connect to gateway", "error", err)
		os.Exit(1)
	}
	defer conn.Close()

	gatewayClient := gatewayv1.NewGatewayServiceClient(conn)
	searchClient := searchv1.NewSearchServiceClient(conn)
	assertionsClient := assertionsv1.NewAssertionsServiceClient(conn)
	contentClient := contentv1.NewContentProcessorServiceClient(conn)
	threadsClient := threadsv1.NewThreadsServiceClient(conn)
	aiClient := aiv1.NewAICoordinatorServiceClient(conn)
	projectClient := projectv1.NewProjectServiceClient(conn)
	entityClient := entityv1.NewEntityManagementServiceClient(conn)
	relationshipClient := relationshipv1.NewRelationshipServiceClient(conn)
	ingestClient := ingestv1.NewIngestServiceClient(conn)
	pipelineClient := pipelinev1.NewPipelineServiceClient(conn)
	qualityClient := qualityv1.NewQualityServiceClient(conn)
	reviewClient := reviewv1.NewReviewServiceClient(conn)
	glossaryClient := glossaryv1.NewGlossaryServiceClient(conn)
	topicClient := topicv1.NewTopicServiceClient(conn)
	conversationClient := conversationv1.NewConversationServiceClient(conn)

	// Define available toolsets — all wired to gRPC backends.
	toolsets := []*Toolset{
		searchToolset(searchClient, assertionsClient, contentClient, threadsClient),
		knowledgeToolset(aiClient, projectClient, assertionsClient),
		entitiesToolset(entityClient, relationshipClient),
		contentToolset(ingestClient, contentClient, pipelineClient, conversationClient),
		opsToolset(pipelineClient, searchClient, qualityClient),
		workflowToolset(reviewClient, glossaryClient, topicClient),
	}

	// Count total tools across all toolsets.
	var totalTools int
	for _, ts := range toolsets {
		totalTools += len(ts.Tools)
	}

	// Build the toolset manager first so we can get its hooks.
	tm := NewToolsetManager(nil, toolsets) // mcpServer set below after hooks are wired
	tm.RegisterHooks()

	mcpServer := server.NewMCPServer(
		"penfold",
		"0.1.0",
		server.WithToolCapabilities(true),
		server.WithHooks(tm.Hooks()),
		server.WithInstructions(serverInstructions),
	)

	// Wire the server back into the manager now that it exists.
	tm.mcpServer = mcpServer

	// Register global tools (meta-tools + health).
	tm.RegisterMetaTools()
	registerHealthTool(mcpServer, gatewayClient)
	metaToolCount := 5 // 4 meta-tools + health

	httpServer := server.NewStreamableHTTPServer(mcpServer)

	slog.Info("penfold-mcp starting",
		"listen", listenAddr,
		"gateway", gatewayAddr,
		"toolsets", len(toolsets),
		"tools_total", totalTools,
		"meta_tools", metaToolCount,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	go func() {
		if err := httpServer.Start(listenAddr); err != nil {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("shutting down")
	if err := httpServer.Shutdown(context.Background()); err != nil {
		slog.Error("shutdown error", "error", err)
	}
}

func boolPtr(b bool) *bool { return &b }
