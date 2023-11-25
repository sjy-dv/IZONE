package main

import (
	"os"
	"os/signal"
	"syscall"

	"github.com/sjy-dv/kslack/internal/role"
	"github.com/sjy-dv/kslack/k8s"
	"github.com/sjy-dv/kslack/pkg/loader"
	"github.com/sjy-dv/kslack/pkg/log"
	"github.com/sjy-dv/kslack/pkg/slack"
)

func main() {
	sigChan := make(chan os.Signal, 1)
	slack.SlackLoad()
	err := k8s.ConfigK8s("windows")
	if err != nil {
		log.Errorf("failed to configure k8s client: %v", err)
		os.Exit(1)
	}
	watchItem, err := loader.LoadEnv("./webhook.yaml")
	if err != nil {
		log.Errorf("ENV LOAD ERROR : ", err)
		os.Exit(1)
	}
	if err := role.SetRole(watchItem); err != nil {
		log.Errorf("Configuring role : %v", err)
		os.Exit(1)
	}
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}
