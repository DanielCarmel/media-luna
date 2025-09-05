package downloader

import (
	"errors"
	"net/http"
	"strings"
)

type YouTube struct {
	YouTubeId	string
	Url 		string
	HTTPClient  *http.Client
}

// NewDB creates a new DB instance with the given configuration.
func NewYoutube(url string) (*YouTube, error) {
	trackPattern := `^https:\/\/www\.youtube\.com\/watch\?v=[a-zA-Z0-9_-]{11}$`
	if !isUrlValid(url, trackPattern) {
		return nil, errors.New("invalid track url")
	}
	
	id := getYoutubeID(url)
	return &YouTube{Url: url, YouTubeId: id}, nil
}

func (y *YouTube) GetTrack() (int, error) {
	return 1, nil
}

func (y *YouTube) GetPlaylist(url string, path string) (int, error) {
	return 1, nil
}

func  (y *YouTube) GetAlbum(url string, path string) (int, error) {
	return 1, nil
}

// getYoutubeID extracts the YouTube video ID from a given URL.
// The function expects the URL to contain the "watch" keyword followed by the video ID.
// If the URL does not contain a valid video ID, an empty string is returned.
//
// Parameters:
//   url (string): The YouTube URL from which to extract the video ID.
//
// Returns:
//   string: The extracted YouTube video ID or an empty string if the ID is not found.
func getYoutubeID(url string) string {
	parts := strings.Split(url, "/")
	var id_part string
	
	for _, p := range parts {
		if strings.HasPrefix(p, "watch") {
			id_part = p
			break
		}
	}

	idParts := strings.Split(id_part, "=")
	if len(idParts) < 2 {
		return ""
	}
	return idParts[1]
}

func downloadSong(stack string, path string) {
	// TODO: check if song exists in DB

}

// func getSongInfo(url string) string {
// 	newCtx := context.Background()
// 	httpClient := http.Client{Transport: &http.Transport{
// 		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
// 	}}

// 	data := innertubeRequest{
// 		VideoID:        getgetYoutubeID(url),
// 		Context:        prepareInnertubeContext(*c.client),
// 		ContentCheckOK: true,
// 		RacyCheckOk:    true,
// 		// Params:                   playerParams,
// 		PlaybackContext: &playbackContext{
// 			ContentPlaybackContext: contentPlaybackContext{
// 				// SignatureTimestamp: sts,
// 				HTML5Preference: "HTML5_PREF_WANTS",
// 			},
// 		},
// 	}
// 	return c.httpPostBodyBytes(ctx, "https://www.youtube.com/youtubei/v1/player?key="+c.client.key, data)
// }