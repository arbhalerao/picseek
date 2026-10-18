package main

import (
	"net/url"
	"os"
)

func hyperlink(path, text string) string {
	host, _ := os.Hostname()
	target := (&url.URL{Scheme: "file", Host: host, Path: path}).String()
	return "\x1b]8;;" + target + "\x1b\\" + text + "\x1b]8;;\x1b\\"
}
