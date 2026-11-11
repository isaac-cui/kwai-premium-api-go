package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	premium "github.com/isaac-cui/kwai-premium-api-go"
)

func main() {
	kind := flag.String("kind", "image", "image or video")
	path := flag.String("file", "", "local media file")
	format := flag.String("type", "jpg", "jpg/jpeg/png/webp for image; mp4/mov for video")
	wait := flag.Bool("wait", false, "wait for successful video transcoding")
	timeout := flag.Duration("timeout", 30*time.Minute, "overall workflow deadline")
	flag.Parse()
	if *path == "" || (*kind != "image" && *kind != "video") {
		fmt.Fprintln(os.Stderr, "usage: upload -kind image|video -file PATH -type FORMAT [-wait]")
		os.Exit(2)
	}
	client, err := premium.New(premium.Config{APIKey: os.Getenv("KWAI_PREMIUM_API_KEY")})
	if err != nil {
		fail(err)
	}
	file, err := os.Open(*path)
	if err != nil {
		fail(err)
	}
	defer file.Close()
	ctx, cancel := context.WithTimeout(context.Background(), *timeout)
	defer cancel()
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	if *kind == "image" {
		response, err := client.Images.Upload(ctx, file, *format)
		if err != nil {
			fail(err)
		}
		if err = encoder.Encode(response.Data); err != nil {
			fail(err)
		}
	} else {
		response, err := client.Videos.Upload(ctx, file, *format)
		if err != nil {
			fail(err)
		}
		if err = encoder.Encode(response.Data); err != nil {
			fail(err)
		}
		if *wait {
			status, err := client.Videos.WaitForTranscode(ctx, response.Data.VideoID, premium.PollOptions{})
			if err != nil {
				fail(err)
			}
			if err = encoder.Encode(status.Data); err != nil {
				fail(err)
			}
		}
	}
}
func fail(err error) { fmt.Fprintln(os.Stderr, err); os.Exit(1) }
