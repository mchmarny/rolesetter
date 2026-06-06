package node

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/mchmarny/rolesetter/pkg/logger"
	"go.uber.org/zap"
)

const (
	envRoleLabel        = "ROLE_LABEL"
	envRoleLabelReplace = "ROLE_LABEL_REPLACE"
	envServerPort       = "SERVER_PORT"
	envNamespace        = "NAMESPACE"
	defaultServerPort   = "8080"
)

// InformNodeRoles is the controller entrypoint invoked by main. It wires
// signal handling, parses configuration from the process environment, and
// runs the Informer until SIGINT/SIGTERM or fatal error.
func InformNodeRoles() {
	if err := run(); err != nil {
		// run() owns logger sync, so emit to stderr-equivalent before exit.
		zap.NewExample().Error("controller terminated", zap.Error(err))
		os.Exit(1)
	}
}

func run() error {
	log := logger.GetLogger()
	defer func() { _ = log.Sync() }()

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	go func() {
		<-ctx.Done()
		log.Info("shutdown signal received")
	}()

	opts, err := optionsFromEnv(log)
	if err != nil {
		log.Error("invalid configuration", zap.Error(err))
		return err
	}

	inf, err := NewInformer(opts...)
	if err != nil {
		log.Error("failed to create informer", zap.Error(err))
		return err
	}

	if err := inf.Inform(ctx); err != nil {
		log.Error("failed to run informer", zap.Error(err))
		return err
	}
	return nil
}

// optionsFromEnv reads controller configuration from the process
// environment and returns a slice of functional options. Returns an error
// for any missing or malformed value so the caller fails fast.
func optionsFromEnv(log *zap.Logger) ([]Option, error) {
	roleLabel := os.Getenv(envRoleLabel)
	if roleLabel == "" {
		return nil, fmt.Errorf("environment variable %s is not set", envRoleLabel)
	}

	portStr := os.Getenv(envServerPort)
	if portStr == "" {
		portStr = defaultServerPort
	}
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		return nil, fmt.Errorf("invalid %s %q: %w", envServerPort, portStr, errors.Join(err, errors.New("must be a positive integer")))
	}

	replace, err := parseBoolish(os.Getenv(envRoleLabelReplace))
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envRoleLabelReplace, err)
	}

	namespace := strings.TrimSpace(os.Getenv(envNamespace))

	opts := []Option{
		WithLogger(log),
		WithLabel(roleLabel),
		WithPort(port),
		WithReplace(replace),
	}
	if namespace != "" {
		opts = append(opts, WithNamespace(namespace))
	}
	return opts, nil
}

// parseBoolish accepts the canonical strconv.ParseBool values plus "yes"/"no"
// for operator ergonomics. Empty string is treated as false.
func parseBoolish(s string) (bool, error) {
	v := strings.TrimSpace(strings.ToLower(s))
	if v == "" {
		return false, nil
	}
	switch v {
	case "yes", "y":
		return true, nil
	case "no", "n":
		return false, nil
	}
	return strconv.ParseBool(v)
}
