package downloader

import (
	"errors"
	"strings"
)

type AudioDownloader interface{
	GetTrack() (int, error)
	GetPlaylist(url string, path string) (int, error)
	GetAlbum(url string, path string) (int, error)
}

func NewAudioDownloader(url string) (*YouTube, error) {
    if strings.Contains(url, "youtube") {
		yt, err := NewYoutube(url)
		if err != nil {
			return nil, err
		}
		return yt, nil
	}

	return nil, errors.New("downloader not implemented")
}