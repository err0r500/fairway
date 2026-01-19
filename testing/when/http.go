package when

import (
	"bytes"
	"encoding/json"
	"net/http"
)

func HttpPostJSON[T json.Marshaler](url string, t *T) (*http.Response, error) {
	postBody, err := json.Marshal(t)
	if err != nil {
		return nil, err
	}

	//Leverage Go's HTTP Post function to make request
	return http.Post(url, "application/json", bytes.NewBuffer(postBody))
}
