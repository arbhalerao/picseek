package indexer

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rwcarlsen/goexif/exif"
)

var filenameTime = regexp.MustCompile(`(\d{4})[-_.]?(\d{2})[-_.]?(\d{2})[ _T-]*(?:at )?(\d{2})[-_.:]?(\d{2})[-_.:]?(\d{2})`)

func takenAt(path string, mtime time.Time) time.Time {
	if t, ok := exifTime(path); ok {
		return t
	}
	if t, ok := filenameTimestamp(filepath.Base(path)); ok {
		return t
	}
	return mtime
}

func exifTime(path string) (time.Time, bool) {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".jpg", ".jpeg", ".tif", ".tiff":
	default:
		return time.Time{}, false
	}
	f, err := os.Open(path)
	if err != nil {
		return time.Time{}, false
	}
	defer f.Close()
	x, err := exif.Decode(f)
	if err != nil {
		return time.Time{}, false
	}
	t, err := x.DateTime()
	if err != nil || !plausible(t) {
		return time.Time{}, false
	}
	return t, true
}

func filenameTimestamp(name string) (time.Time, bool) {
	m := filenameTime.FindStringSubmatch(name)
	if m == nil {
		return time.Time{}, false
	}
	parts := make([]int, 6)
	for i := range parts {
		parts[i], _ = strconv.Atoi(m[i+1])
	}
	t := time.Date(parts[0], time.Month(parts[1]), parts[2], parts[3], parts[4], parts[5], 0, time.Local)
	if t.Month() != time.Month(parts[1]) || t.Day() != parts[2] || t.Hour() != parts[3] || !plausible(t) {
		return time.Time{}, false
	}
	return t, true
}

func plausible(t time.Time) bool {
	return t.Year() >= 1990 && t.Before(time.Now().Add(48*time.Hour))
}
