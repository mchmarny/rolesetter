package main

import (
	"context"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/mchmarny/rolesetter/pkg/log"
	"github.com/mchmarny/rolesetter/pkg/node"
	"go.uber.org/zap"
)

func main() {
	logger := log.GetLogger()
	defer func() { _ = logger.Sync() }()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		logger.Info("shutdown signal received", zap.String("signal", sig.String()))
		cancel()
	}()

	// Read environment variables for role label and server port
	roleLabel := os.Getenv("ROLE_LABEL")
	if roleLabel == "" {
		logger.Fatal("ROLE_LABEL environment variable is not set")
	}

	serverPort := os.Getenv("SERVER_PORT")
	if serverPort == "" {
		serverPort = "8080" // Default port if not set
	}

	// parse integer port
	port, err := strconv.Atoi(serverPort)
	if err != nil || port <= 0 {
		logger.Fatal("invalid SERVER_PORT environment variable", zap.Error(err))
	}

	// Create a new informer instance
	inf, err := node.NewInformer(logger, roleLabel, port)
	if err != nil {
		logger.Fatal("failed to create informer", zap.Error(err))
	}

	// Run the informer
	if err := inf.Inform(ctx); err != nil {
		logger.Fatal("failed to run informer", zap.Error(err))
	}
}
