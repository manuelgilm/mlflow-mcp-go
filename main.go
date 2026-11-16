package main

import (
	"context"
	"fmt"
	"github.com/manuelgilm/mlflow-mcp-go/internal/server"
	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	
	"log"
)

func main() {
	fmt.Println("Creating MCP Server")
	const baseURL = "http://localhost:5000"
	context := context.Background()
	mlflowClient := mlflow.NewClient(baseURL)
	handler := server.New(mlflowClient)
	srv := mcp.NewServer(&mcp.Implementation{Name: "MLflow-MCP-GO", Version: "1.0"}, nil)
	handler.RegisterTools(srv)

	if err := srv.Run(context, &mcp.StdioTransport{}); err != nil {

		log.Fatal(err)
	}
}
