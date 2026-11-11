package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"time"

	premium "github.com/isaac-cui/kwai-premium-api-go"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: create-drama request.json")
		os.Exit(2)
	}
	body, err := os.ReadFile(os.Args[1])
	if err != nil {
		fail(err)
	}
	var request premium.CreateDramaRequest
	if err = json.Unmarshal(body, &request); err != nil {
		fail(err)
	}
	if err = request.Validate(); err != nil {
		fail(err)
	}
	client, err := premium.New(premium.Config{APIKey: os.Getenv("KWAI_PREMIUM_API_KEY")})
	if err != nil {
		fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	response, err := client.Dramas.Create(ctx, request)
	if err != nil {
		fail(err)
	}
	if err = json.NewEncoder(os.Stdout).Encode(response.Data); err != nil {
		fail(err)
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
