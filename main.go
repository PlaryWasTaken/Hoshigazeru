package main

import (
	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/PlaryWasTaken/Hoshigazeru/web"
	"log/slog"
	"os"
	"time"
)

func main() {
	client := Client.CreateClient(time.Second*5, time.Hour)
	client.Start()

	err := web.Start(client)
	if err != nil {
		slog.Error("Failed to start web server", slog.Any("error", err))
		os.Exit(1)
		return
	}
}
