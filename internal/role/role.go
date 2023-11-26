package role

import (
	"time"

	"github.com/sjy-dv/IZONE/hub"
	"github.com/sjy-dv/IZONE/k8s"
	"github.com/sjy-dv/IZONE/pkg/loader"
	"github.com/sjy-dv/IZONE/pkg/log"
)

const (
	Pod              string = "Pod"
	Deployment       string = "Deployment"
	StatefulSet      string = "StatefulSet"
	PersistentVolume string = "PersistentVolume"
	Node             string = "Node"
	MySQL            string = "MySQL"
)

/*
Logging Level
1 <- : Checks all statuses at set intervals even in normal conditions, including increases in usage.
2 <- : Only checks for increased resource usage, changes in the number of replicas, and pending errors.
3 <- : Only checks for pending errors.
*/
func SetRole(items map[string]loader.IZONEConfig) error {

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
		if watchConfig.Type == Deployment {
			ensureRole(watchName, &watchConfig)
			k8s.RegisterDeployments(&k8s.Deployment{
				Label:        watchConfig.Label,
				Namespace:    watchConfig.Namespace,
				Logging:      watchConfig.Logging,
				LoggingLevel: watchConfig.LoggingLevel,
				ScaleUp: func() bool {
					if watchConfig.ScaleOut {
						return false
					}
					return watchConfig.ScaleUp
				}(),
				ScaleOut:                    watchConfig.ScaleOut,
				LimitMemoryUsage:            watchConfig.LimitMemoryUsage,
				LimitCpuUsage:               watchConfig.LimitCpuUsage,
				Interval:                    time.Duration(watchConfig.Interval) * time.Second,
				ScaleDownInterval:           watchConfig.ScaleDownInterval,
				MinReplicas:                 watchConfig.MinReplicas,
				MaxReplicas:                 watchConfig.MaxReplicas,
				MaxMemoryPercentBeforeScale: watchConfig.MaxMemoryPercentBeforeScale,
				MaxCpuPercentBeforeScale:    watchConfig.MaxCpuPercentBeforeScale,
				SlackUrl:                    watchConfig.SlackUrl,
			})
		}
		if watchConfig.Type == StatefulSet {
			ensureRole(watchName, &watchConfig)
			k8s.RegisterStatefulsets(&k8s.Statefulset{
				Label:        watchConfig.Label,
				Namespace:    watchConfig.Namespace,
				Logging:      watchConfig.Logging,
				LoggingLevel: watchConfig.LoggingLevel,
				ScaleUp: func() bool {
					if watchConfig.ScaleOut {
						return false
					}
					return watchConfig.ScaleUp
				}(),
				ScaleOut:                    watchConfig.ScaleOut,
				LimitMemoryUsage:            watchConfig.LimitMemoryUsage,
				LimitCpuUsage:               watchConfig.LimitCpuUsage,
				Interval:                    time.Duration(watchConfig.Interval) * time.Second,
				ScaleDownInterval:           watchConfig.ScaleDownInterval,
				MinReplicas:                 watchConfig.MinReplicas,
				MaxReplicas:                 watchConfig.MaxReplicas,
				MaxMemoryPercentBeforeScale: watchConfig.MaxMemoryPercentBeforeScale,
				MaxCpuPercentBeforeScale:    watchConfig.MaxCpuPercentBeforeScale,
				SlackUrl:                    watchConfig.SlackUrl,
			})
		}
		if watchConfig.Type == Node {
			k8s.RegisterNode(&k8s.Node{
				Interval: time.Duration(watchConfig.Interval) * time.Second,
				SlackUrl: watchConfig.SlackUrl,
			})
		}
		if watchConfig.Type == MySQL {
			hub.MysqlConnector(&hub.DBConnector{
				User:     watchConfig.User,
				Password: watchConfig.Password,
				Host:     watchConfig.Host,
				Port:     watchConfig.Port,
				Label:    watchConfig.Label,
			})
		}
	}
	return nil
}

func ensureRole(label string, cfg *loader.IZONEConfig) {
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
		if cfg.ScaleOut && cfg.ScaleUp {
			log.Warnf("deployment %s. The options for scale up and scale out cannot be selected simultaneously. The option will automatically change to prioritize scale out.", label)
		}
		if cfg.MinReplicas < 1 {
			log.Warnf("deployment %s. The minimum number of replicas cannot be less than 1. It will be automatically set to 1.", label)
		}
		if cfg.MaxReplicas > 9 {
			log.Warnf("deployment %s. The maximum number of replicas cannot be more than 9. It will be automatically set to 9.", label)
		}
		break
	case StatefulSet:
		if cfg.ScaleOut && cfg.ScaleUp {
			log.Warnf("statefulset %s. The options for scale up and scale out cannot be selected simultaneously. The option will automatically change to prioritize scale out.", label)
		}
		if cfg.MinReplicas < 1 {
			log.Warnf("statefulset %s. The minimum number of replicas cannot be less than 1. It will be automatically set to 1.", label)
		}
		if cfg.MaxReplicas > 9 {
			log.Warnf("statefulset %s. The maximum number of replicas cannot be more than 9. It will be automatically set to 9.", label)
		}
		break
	case PersistentVolume:
		break
	}
}
