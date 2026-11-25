package main

import (
	"context"
	"log"
	"os"

	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/manuelgilm/mlflow-mcp-go/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	baseURL := os.Getenv("MLFLOW_TRACKING_URI")
	if baseURL == "" {
		baseURL = "http://localhost:5000"
	}

	ctx := context.Background()
	mlflowClient := mlflow.NewClient(baseURL)
	handler := server.New(mlflowClient)
	srv := mcp.NewServer(&mcp.Implementation{Name: "mlflow-mcp", Version: "0.2.1"}, nil) // keep in sync with VERSION
	handler.RegisterTools(srv)
	handler.RegisterResources(srv)
	handler.RegisterPrompts(srv)

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
