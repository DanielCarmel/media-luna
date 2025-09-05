package downloader

import (
	"regexp"
)

func isUrlValid(url, pattern string) bool {
	match, _ := regexp.MatchString(pattern, url)
	return match
}