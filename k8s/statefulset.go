package k8s

import (
	"sync"
	"time"
)

type Statefulset struct {
	Label                       string
	Namespace                   string
	Logging                     bool
	LoggingLevel                int
	ScaleUp                     bool
	LimitMemoryUsage            int
	LimitCpuUsage               int
	SlackUrl                    string
	Interval                    time.Duration
	ScaleOut                    bool `yaml:"scale_out"`
	ScaleDownInterval           int  `yaml:"scale_down_interval"`
	MinReplicas                 int  `yaml:"min_replicas"`
	MaxReplicas                 int  `yaml:"max_replicas"`
	MaxMemoryPercentBeforeScale int  `yaml:"max_memory_percent_before_scale"`
	MaxCpuPercentBeforeScale    int  `yaml:"max_cpu_percent_before_scale"`
	pods                        []*Pod
	timer                       *time.Ticker
}

type statefulsetGroup struct {
	mu     sync.Mutex
	groups []*statefulsetGroup
}

var statefulsets *statefulsetGroup
