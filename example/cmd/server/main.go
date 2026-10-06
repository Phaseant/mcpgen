package main

import (
	"context"
	"log"
	"os"
	"os/signal"

	"github.com/phaseant/mcpgen/example/internal/mcpapi"
	"github.com/phaseant/mcpgen/example/internal/mcpserver"
	"github.com/phaseant/mcpgen/example/internal/service"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	server, err := mcpapi.NewServer(mcpserver.New(&service.Pets{}))
	if err == nil {
		err = server.Run(ctx)
	}
	if err != nil && ctx.Err() == nil {
		log.Fatal(err)
	}
}
