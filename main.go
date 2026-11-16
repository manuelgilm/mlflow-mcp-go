package main

import (
	"context"
	"fmt"
	// "github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"log"
)

func main() {
	fmt.Println("Creating MCP Server")
	context := context.Background()
	server := mcp.NewServer(&mcp.Implementation{Name: "MLflow-MCP-GO", Version: "1.0"}, nil)
	if err := server.Run(context, &mcp.StdioTransport{}); err != nil {
		log.Fatal(err)
	}
}
