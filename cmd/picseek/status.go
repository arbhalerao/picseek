package main

import (
	"context"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
)

func (a *app) statusCommand() *cobra.Command {
	var stats bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show watched folders, indexing progress and service health",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := a.connect(); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			resp, err := a.client.Status(ctx, &seekdv1.StatusRequest{})
			if err != nil {
				return err
			}
			st := resp.GetStatus()
			fmt.Printf("%-9s running\n", "seekd")
			for _, svc := range st.GetServices() {
				state := fmt.Sprintf("%d/%d workers healthy", svc.GetHealthy(), svc.GetTotal())
				if svc.GetTotal() == 1 {
					state = map[bool]string{true: "healthy", false: "unavailable"}[svc.GetHealthy() == 1]
				}
				fmt.Printf("%-9s %s\n", svc.GetName(), state)
			}
			for i, f := range st.GetFolders() {
				label := ""
				if i == 0 {
					label = "folders"
				}
				fmt.Printf("%-9s %s\n", label, tildify(f))
			}
			queued := st.GetQueuedLive() + st.GetQueuedBackfill()
			fmt.Printf("%-9s %d\n", "indexed", st.GetIndexed())
			fmt.Printf("%-9s %d (new %d, backfill %d)\n", "queued", queued, st.GetQueuedLive(), st.GetQueuedBackfill())
			fmt.Printf("%-9s %d (embeddings and classification, after OCR)\n", "enriching", st.GetQueuedEnrich())
			fmt.Printf("%-9s %d\n", "failed", st.GetFailed())
			if st.GetSkipped() > 0 {
				fmt.Printf("%-9s %d (larger than index.max_file_mb)\n", "skipped", st.GetSkipped())
			}
			if stats {
				fmt.Printf("%-9s %.2f images/s (last 30 s)\n", "rate", st.GetImagesPerSecond())
				if queued > 0 {
					fmt.Printf("%-9s %s\n", "eta", st.GetEta().AsDuration().Round(time.Second))
				}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&stats, "stats", false, "include throughput and ETA")
	return cmd
}
