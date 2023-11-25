package slack

import (
	"bytes"
	"encoding/json"
	"net/http"

	"github.com/sjy-dv/kslack/pkg/log"
)

var Channel = make(chan *KSlackForm)

func SlackLoad() {
	go publisher()
}

func publisher() {
	for {
		select {
		case c := <-Channel:
			apiWebHook(c)
		}
	}
}
func apiWebHook(data *KSlackForm) {

	b, err := json.Marshal(kslackmsgform(data))
	if err != nil {
		log.Errorf("JSON marshal error: %v", err)
	}
	req, err := http.NewRequest("POST", data.WebHookUrl, bytes.NewReader(b))
	if err != nil {
		log.Errorf("JSON request error: %v", err)
	}
	req.Header.Add("Content-Type", "application/json")
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Errorf("CLIENT DO ERROR: %v", err)
	}
	defer resp.Body.Close()
}
