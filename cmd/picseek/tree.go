package main

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	seekdv1 "github.com/arbhalerao/picseek/gen/go/picseek/seekd/v1"
)

type dirNode struct {
	name     string
	children map[string]*dirNode
	files    []*seekdv1.Hit
	best     int32
}

func newDir(name string) *dirNode {
	return &dirNode{name: name, children: make(map[string]*dirNode), best: 1 << 30}
}

func printTree(w io.Writer, hits []*seekdv1.Hit, why bool) {
	anchors := newDir("")
	for _, h := range hits {
		parts := splitPath(tildify(h.GetPath()))
		node := anchors
		node.best = min(node.best, h.GetRank())
		for _, part := range parts[:len(parts)-1] {
			child, ok := node.children[part]
			if !ok {
				child = newDir(part)
				node.children[part] = child
			}
			child.best = min(child.best, h.GetRank())
			node = child
		}
		node.files = append(node.files, h)
	}

	var tops []*dirNode
	for _, anchor := range sortedChildren(anchors) {
		if len(anchor.files) > 0 {
			tops = append(tops, anchor)
			continue
		}
		for _, child := range sortedChildren(anchor) {
			child.name = joinName(anchor.name, child.name)
			tops = append(tops, child)
		}
	}
	slices.SortFunc(tops, func(a, b *dirNode) int { return int(a.best - b.best) })

	for _, top := range tops {
		top = collapse(top)
		fmt.Fprintln(w, top.name)
		printChildren(w, top, "", why)
	}
}

func printChildren(w io.Writer, dir *dirNode, prefix string, why bool) {
	type entry struct {
		rank int32
		dir  *dirNode
		file *seekdv1.Hit
	}
	var entries []entry
	for _, child := range dir.children {
		entries = append(entries, entry{rank: child.best, dir: child})
	}
	for _, f := range dir.files {
		entries = append(entries, entry{rank: f.GetRank(), file: f})
	}
	slices.SortFunc(entries, func(a, b entry) int { return int(a.rank - b.rank) })

	for i, e := range entries {
		branch, indent := "├── ", "│   "
		if i == len(entries)-1 {
			branch, indent = "└── ", "    "
		}
		if e.file != nil {
			line := fmt.Sprintf("[%d] %s", e.file.GetRank(), filepath.Base(e.file.GetPath()))
			if why {
				line += "  (" + kindName(e.file.GetBest()) + ")"
			}
			fmt.Fprintln(w, prefix+branch+line)
			continue
		}
		child := collapse(e.dir)
		fmt.Fprintln(w, prefix+branch+child.name)
		printChildren(w, child, prefix+indent, why)
	}
}

func collapse(d *dirNode) *dirNode {
	for len(d.files) == 0 && len(d.children) == 1 {
		var only *dirNode
		for _, c := range d.children {
			only = c
		}
		merged := *only
		merged.name = joinName(d.name, only.name)
		d = &merged
	}
	return d
}

func sortedChildren(d *dirNode) []*dirNode {
	out := make([]*dirNode, 0, len(d.children))
	for _, c := range d.children {
		out = append(out, c)
	}
	slices.SortFunc(out, func(a, b *dirNode) int { return int(a.best - b.best) })
	return out
}

func splitPath(p string) []string {
	if strings.HasPrefix(p, "/") {
		return append([]string{"/"}, strings.Split(strings.TrimPrefix(p, "/"), "/")...)
	}
	return strings.Split(p, "/")
}

func joinName(parent, child string) string {
	if parent == "/" {
		return "/" + child
	}
	return parent + "/" + child
}

func summary(hits []*seekdv1.Hit) string {
	dirs := make(map[string]struct{})
	for _, h := range hits {
		dirs[filepath.Dir(h.GetPath())] = struct{}{}
	}
	return fmt.Sprintf("%d %s in %d %s", len(hits), plural(len(hits), "match", "matches"), len(dirs), plural(len(dirs), "folder", "folders"))
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func tildify(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if rest, ok := strings.CutPrefix(p, home+"/"); ok {
		return "~/" + rest
	}
	return p
}
