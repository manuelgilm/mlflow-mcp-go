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

	run, err := mlflowClient.GetRun(context, "7a5e742f4ee64735973660a8eb211b7a")
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println("Printing Run Info")
	fmt.Println(run.Info)
	fmt.Println("Printing Run Data")
	fmt.Println(run.Data)

}
