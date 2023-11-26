package main

import (
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/joho/godotenv"
	"github.com/sjy-dv/IZONE/internal/role"
	"github.com/sjy-dv/IZONE/k8s"
	"github.com/sjy-dv/IZONE/pkg/loader"
	"github.com/sjy-dv/IZONE/pkg/log"
	"github.com/sjy-dv/IZONE/pkg/slack"
)

func init() {
	godotenv.Load()
	os.Setenv("TZ", "UTC")
	time.Local = time.UTC
}

func main() {
	sigChan := make(chan os.Signal, 1)
	slack.SlackLoad()
	err := k8s.ConfigK8s("windows")
	if err != nil {
		log.Errorf("failed to configure k8s client: %v", err)
		os.Exit(1)
	}
	log.Info("K8S client started")
	log.Info("K8S metrics started")
	watchItem, err := loader.LoadEnv("./webhook.yaml")
	if err != nil {
		log.Errorf("ENV LOAD ERROR : ", err)
		os.Exit(1)
	}
	log.Info("Successfully get item spec file")
	if err := role.SetRole(watchItem); err != nil {
		log.Errorf("Configuring role : %v", err)
		os.Exit(1)
	}
	log.Info("The role has been successfully assigned.")
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan
}
