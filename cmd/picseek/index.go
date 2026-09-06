package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
)

func (a *app) indexCommand() *cobra.Command {
	var forget string
	var retryFailed, noWait bool
	cmd := &cobra.Command{
		Use:   "index [folders...]",
		Short: "Watch folders and index their images; shows progress until the queue is empty",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := a.connect(); err != nil {
				return err
			}
			ctx, stop := signal.NotifyContext(cmd.Context(), os.Interrupt)
			defer stop()

			if forget != "" {
				abs, err := filepath.Abs(forget)
				if err != nil {
					return err
				}
				resp, err := a.client.ForgetFolder(ctx, &seekdv1.ForgetFolderRequest{Path: abs})
				if err != nil {
					return err
				}
				fmt.Printf("stopped watching %s (%d paths removed)\n", tildify(abs), resp.GetRemovedPaths())
				return nil
			}
			if retryFailed {
				resp, err := a.client.RetryFailed(ctx, &seekdv1.RetryFailedRequest{})
				if err != nil {
					return err
				}
				fmt.Printf("re-queued %d failed images\n", resp.GetRequeued())
			}
			if len(args) > 0 {
				paths := make([]string, len(args))
				for i, arg := range args {
					abs, err := filepath.Abs(arg)
					if err != nil {
						return err
					}
					paths[i] = abs
				}
				callCtx, cancel := context.WithTimeout(ctx, callTimeout)
				resp, err := a.client.AddFolders(callCtx, &seekdv1.AddFoldersRequest{Paths: paths})
				cancel()
				if err != nil {
					return err
				}
				for _, p := range resp.GetAdded() {
					fmt.Printf("watching %s\n", tildify(p))
				}
			}
			if noWait {
				return nil
			}
			return a.followProgress(ctx)
		},
	}
	cmd.Flags().StringVar(&forget, "forget", "", "stop watching a folder and remove its entries")
	cmd.Flags().BoolVar(&retryFailed, "retry-failed", false, "re-queue images that failed repeatedly")
	cmd.Flags().BoolVar(&noWait, "no-wait", false, "return immediately instead of showing progress")
	return cmd
}

func (a *app) followProgress(ctx context.Context) error {
	stream, err := a.client.WatchStatus(ctx, &seekdv1.WatchStatusRequest{})
	if err != nil {
		return err
	}
	tty := isTerminal(os.Stdout)
	started := time.Now()
	for {
		resp, err := stream.Recv()
		if err != nil {
			if ctx.Err() != nil {
				fmt.Println("\nindexing continues in the background; check with `picseek status`")
				return nil
			}
			return err
		}
		st := resp.GetStatus()
		queued := st.GetQueuedLive() + st.GetQueuedBackfill()
		line := fmt.Sprintf("indexed %d of %d", st.GetIndexed(), st.GetIndexed()+queued)
		if rate := st.GetImagesPerSecond(); rate > 0 && queued > 0 {
			line += fmt.Sprintf(" · %.1f img/s · ETA %s", rate, st.GetEta().AsDuration().Round(time.Second))
		}
		if failed := st.GetFailed(); failed > 0 {
			line += fmt.Sprintf(" · %d failed", failed)
		}
		if tty {
			fmt.Printf("\r\033[K%s", line)
		}
		if queued == 0 {
			if tty {
				fmt.Println()
			} else {
				fmt.Println(line)
			}
			fmt.Printf("up to date (%s)\n", time.Since(started).Round(time.Second))
			return nil
		}
	}
}

func isTerminal(f *os.File) bool {
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
