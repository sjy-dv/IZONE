package slack

import (
	"fmt"
	"time"
)

type KSlackForm struct {
	Text       string
	Level      string
	WebHookUrl string
}

func kslackmsgform(k *KSlackForm) map[string]interface{} {
	return map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  "#36a64f",
				"title":  fmt.Sprintf("KSlack Notifications - %s", k.Level),
				"text":   k.Text,
				"footer": "kslack-notify",
				"ts":     time.Now().Unix(),
			},
		},
	}
}
