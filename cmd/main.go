package main

import (
	"os"

	"github.com/sjy-dv/kslack/internal/role"
	"github.com/sjy-dv/kslack/k8s"
	"github.com/sjy-dv/kslack/pkg/loader"
	"github.com/sjy-dv/kslack/pkg/log"
)

func main() {
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
}
