package slack

import (
	"fmt"
	"time"
)

type Level int

const (
	INFO Level = iota
	WARNING
	ERROR
)

type KSlackForm struct {
	Text       string
	Level      Level
	WebHookUrl string
}

var notiLv = []string{"INFO", "WARNING", "ERROR"}

var colorLv = []string{"#36a64f", "#FFFD700", "#FF0000"}

func kslackmsgform(k *KSlackForm) map[string]interface{} {
	return map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  colorLv[k.Level],
				"title":  fmt.Sprintf("KSlack Notifications - %s", notiLv[k.Level]),
				"text":   k.Text,
				"footer": "kslack-notify",
				"ts":     time.Now().Unix(),
			},
		},
	}
}
