package main

import (
	"context"
	"log"

	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/manuelgilm/mlflow-mcp-go/internal/server"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func main() {
	const baseURL = "http://localhost:5000"
	ctx := context.Background()
	mlflowClient := mlflow.NewClient(baseURL)
	handler := server.New(mlflowClient)
	srv := mcp.NewServer(&mcp.Implementation{Name: "MLflow-MCP-GO", Version: "1.0"}, nil)
	handler.RegisterTools(srv)
	handler.RegisterResources(srv)

	if err := srv.Run(ctx, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
