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

type IZONEForm struct {
	Text       string
	Level      Level
	WebHookUrl string
}

var notiLv = []string{"INFO", "WARNING", "ERROR"}

var colorLv = []string{"#36a64f", "#FFFD700", "#FF0000"}

func IZONEmsgform(k *IZONEForm) map[string]interface{} {
	return map[string]interface{}{
		"attachments": []map[string]interface{}{
			{
				"color":  colorLv[k.Level],
				"title":  fmt.Sprintf("IZONE Notifications - %s", notiLv[k.Level]),
				"text":   k.Text,
				"footer": "IZONE-notify",
				"ts":     time.Now().Unix(),
			},
		},
	}
}
