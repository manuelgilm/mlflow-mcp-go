package main

import (
	"context"
	"fmt"
	"github.com/manuelgilm/mlflow-mcp-go/internal/mlflow"
	"log"
)

func main() {
	fmt.Println("Creating mlflow client")
	var baseURL string = "http://localhost:5000"
	context := context.Background()
	mlflowClient := mlflow.NewClient(baseURL)

	run, err := mlflowClient.GetRun(context, "245345")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(run)

}
