package role

import (
	"time"

	"github.com/sjy-dv/kslack/k8s"
	"github.com/sjy-dv/kslack/pkg/loader"
	"github.com/sjy-dv/kslack/pkg/log"
)

const (
	Pod              string = "Pod"
	Deployment       string = "Deployment"
	StatefulSet      string = "StatefulSet"
	PersistentVolume string = "PersistentVolume"
)

/*
Logging Level
1 <- : Checks all statuses at set intervals even in normal conditions, including increases in usage.
2 <- : Only checks for increased resource usage, changes in the number of replicas, and pending errors.
3 <- : Only checks for pending errors.
*/
func SetRole(items map[string]loader.KslackConfig) error {

	for watchName, watchConfig := range items {
		if watchConfig.Type == Pod {
			ensureRole(watchName, &watchConfig)
			k8s.RegisterPods(&k8s.Pod{
				Label:            watchConfig.Label,
				Namespace:        watchConfig.Namespace,
				Logging:          watchConfig.Logging,
				LoggingLevel:     watchConfig.LoggingLevel,
				ScaleUp:          watchConfig.ScaleUp,
				LimitMemoryUsage: watchConfig.LimitMemoryUsage,
				LimitCpuUsage:    watchConfig.LimitCpuUsage,
				SlackUrl:         watchConfig.SlackUrl,
				Interval:         time.Duration(time.Second * time.Duration(watchConfig.Interval)),
			})
		}
	}
	return nil
}

func ensureRole(label string, cfg *loader.KslackConfig) {
	switch cfg.Type {
	case Pod:
		if cfg.ScaleOut {
			log.Warnf("%s is a pod. not support scale out", label)
		}
		if cfg.MinReplicas > 1 {
			log.Warnf("%s is a pod. not support replicas configuration", label)
		}
		break
	case Deployment:
		break
	case StatefulSet:
		break
	case PersistentVolume:
		break
	}
}
