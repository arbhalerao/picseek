package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
)

type jsonHit struct {
	Rank    int      `json:"rank"`
	Path    string   `json:"path"`
	Match   string   `json:"match"`
	Matches []string `json:"matches"`
	Score   float64  `json:"score"`
}

func (a *app) searchCommand() *cobra.Command {
	var limit int
	var why, paths, asJSON bool
	var layerNames []string
	cmd := &cobra.Command{
		Use:   "search <query>",
		Short: "Search indexed images",
		Long: `Search indexed images by the text in them.

Results come in tiers: exact text matches, near matches (OCR misreads and
split words), then matches by meaning and by what the image shows.
Every word must appear somewhere in the image for text matches. Partial words match.
  "exact phrase"   must appear on one line
  -word            exclude images containing word
  folder:name      folder path contains name
  on:week          today, yesterday, week, month, year, march, 2026, 2026-08, 2026-08-01
  since:/until:    same formats as on:
  type:money       money, travel, message, dev, security, document, chart

Flags may appear anywhere; any other argument starting with - is part of the query.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			flags, terms := splitSearchArgs(args)
			fs := cmd.Flags()
			fs.AddFlagSet(cmd.InheritedFlags())
			if err := fs.Parse(flags); err != nil {
				return err
			}
			if help, _ := fs.GetBool("help"); help {
				return cmd.Help()
			}
			if paths && asJSON {
				return errors.New("--paths and --json cannot be used together")
			}
			if len(terms) == 0 {
				return errors.New("missing query; see `picseek search --help`")
			}
			if err := a.connect(); err != nil {
				return err
			}
			ctx, cancel := context.WithTimeout(cmd.Context(), callTimeout)
			defer cancel()
			layers, err := parseLayers(layerNames)
			if err != nil {
				return err
			}
			resp, err := a.client.Search(ctx, &seekdv1.SearchRequest{Query: strings.Join(terms, " "), Limit: int32(limit), Layers: layers})
			if err != nil {
				return err
			}
			hits := resp.GetHits()
			switch {
			case asJSON:
				out := make([]jsonHit, len(hits))
				for i, h := range hits {
					out[i] = jsonHit{Rank: int(h.GetRank()), Path: h.GetPath(), Match: kindName(h.GetBest()), Score: h.GetScore()}
					for _, m := range h.GetMatches() {
						out[i].Matches = append(out[i].Matches, kindName(m))
					}
				}
				enc := json.NewEncoder(os.Stdout)
				enc.SetIndent("", "  ")
				if err := enc.Encode(out); err != nil {
					return err
				}
			case paths:
				for _, h := range hits {
					fmt.Println(h.GetPath())
				}
			default:
				if len(hits) == 0 {
					fmt.Fprintln(os.Stderr, "no matches")
					break
				}
				printTree(os.Stdout, hits, why)
				fmt.Printf("\n%s · %s\n", summary(hits), resp.GetTook().AsDuration().Round(100_000))
			}
			if len(hits) == 0 {
				return errNoResults
			}
			return nil
		},
	}
	cmd.Flags().IntVarP(&limit, "limit", "n", 0, "maximum number of results (default: all)")
	cmd.Flags().BoolVar(&why, "why", false, "show why each file matched")
	cmd.Flags().BoolVar(&paths, "paths", false, "print bare paths, one per line")
	cmd.Flags().BoolVar(&asJSON, "json", false, "print results as JSON")
	cmd.Flags().StringSliceVar(&layerNames, "layers", nil, "search only these layers: exact,near,meaning,visual")
	cmd.Flags().BoolP("help", "h", false, "help for search")
	return cmd
}

func splitSearchArgs(args []string) (flags, terms []string) {
	takesValue := map[string]bool{"-n": true, "--limit": true, "--config": true, "--layers": true}
	boolean := map[string]bool{"--why": true, "--paths": true, "--json": true, "-h": true, "--help": true}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, _, hasValue := strings.Cut(arg, "=")
		switch {
		case arg == "--":
			return flags, append(terms, args[i+1:]...)
		case boolean[arg]:
			flags = append(flags, arg)
		case takesValue[name] && hasValue:
			flags = append(flags, arg)
		case takesValue[arg] && i+1 < len(args):
			flags = append(flags, arg, args[i+1])
			i++
		case strings.HasPrefix(arg, "-n") && len(arg) > 2 && isDigits(arg[2:]):
			flags = append(flags, arg)
		default:
			terms = append(terms, arg)
		}
	}
	return flags, terms
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

func parseLayers(names []string) ([]seekdv1.MatchKind, error) {
	kinds := map[string]seekdv1.MatchKind{
		"exact":   seekdv1.MatchKind_MATCH_KIND_EXACT,
		"near":    seekdv1.MatchKind_MATCH_KIND_NEAR,
		"meaning": seekdv1.MatchKind_MATCH_KIND_MEANING,
		"visual":  seekdv1.MatchKind_MATCH_KIND_VISUAL,
	}
	layers := make([]seekdv1.MatchKind, 0, len(names))
	for _, n := range names {
		k, ok := kinds[strings.ToLower(strings.TrimSpace(n))]
		if !ok {
			return nil, fmt.Errorf("unknown layer %q (use exact, near, meaning, visual)", n)
		}
		layers = append(layers, k)
	}
	return layers, nil
}

func kindName(k seekdv1.MatchKind) string {
	switch k {
	case seekdv1.MatchKind_MATCH_KIND_EXACT:
		return "exact"
	case seekdv1.MatchKind_MATCH_KIND_NEAR:
		return "near"
	case seekdv1.MatchKind_MATCH_KIND_MEANING:
		return "meaning"
	case seekdv1.MatchKind_MATCH_KIND_VISUAL:
		return "visual"
	default:
		return "filter"
	}
}
