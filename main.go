package main

import (
	"log/slog"
	"os"
	"time"

	"github.com/PlaryWasTaken/Hoshigazeru/Client"
	"github.com/PlaryWasTaken/Hoshigazeru/web"
)

func main() {
	client := Client.CreateClient(15*time.Second, time.Hour)
	client.Start()
	if err := web.Start(client); err != nil {
		slog.Error("Web server stopped", slog.Any("error", err))
		os.Exit(1)
	}
}
