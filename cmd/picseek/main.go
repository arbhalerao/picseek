package main

import (
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/spf13/cobra"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
	"github.com/arbhalerao/picseek/internal/config"
)

const callTimeout = 30 * time.Second

var errNoResults = errors.New("no results")

type app struct {
	configPath string
	socket     string
	conn       *grpc.ClientConn
	client     seekdv1.SeekdServiceClient
}

func main() {
	a := &app{}
	root := &cobra.Command{
		Use:           "picseek",
		Short:         "Search your screenshots by what they say and show",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	defaultPath, _ := config.DefaultPath()
	root.PersistentFlags().StringVar(&a.configPath, "config", defaultPath, "path to config.toml")
	root.AddCommand(a.indexCommand(), a.searchCommand(), a.statusCommand())

	err := root.Execute()
	if a.conn != nil {
		a.conn.Close()
	}
	switch {
	case err == nil:
	case errors.Is(err, errNoResults):
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "picseek:", describe(err, a.socket))
		os.Exit(2)
	}
}

func (a *app) connect() error {
	if a.client != nil {
		return nil
	}
	cfg, err := config.Load(a.configPath)
	if err != nil {
		return err
	}
	a.socket = cfg.SocketPath("seekd")
	if _, err := os.Stat(a.socket); err != nil {
		return fmt.Errorf("seekd is not running (no socket at %s); start it with `systemctl --user start seekd` or `make run`", a.socket)
	}
	a.conn, err = grpc.NewClient("unix://"+a.socket, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return err
	}
	a.client = seekdv1.NewSeekdServiceClient(a.conn)
	return nil
}

func describe(err error, socket string) string {
	if s, ok := status.FromError(err); ok {
		if s.Code().String() == "Unavailable" {
			return fmt.Sprintf("cannot reach seekd at %s: %s", socket, s.Message())
		}
		return s.Message()
	}
	return err.Error()
}
