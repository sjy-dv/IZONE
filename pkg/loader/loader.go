package loader

import (
	"os"

	"gopkg.in/yaml.v2"
)

type IZONEConfig struct {
	Type                        string `yaml:"type"`
	Label                       string `yaml:"label"`
	Namespace                   string `yaml:"namespace"`
	Interval                    int    `yaml:"interval"`
	Logging                     bool   `yaml:"logging"`
	LoggingLevel                int    `yaml:"logging_level"`
	ScaleOut                    bool   `yaml:"scale_out"`
	ScaleDownInterval           int    `yaml:"scale_down_interval"`
	MinReplicas                 int    `yaml:"min_replicas"`
	MaxReplicas                 int    `yaml:"max_replicas"`
	MaxMemoryPercentBeforeScale int    `yaml:"max_memory_percent_before_scale"`
	MaxCpuPercentBeforeScale    int    `yaml:"max_cpu_percent_before_scale"`
	ScaleUp                     bool   `yaml:"scale_up"`
	LimitMemoryUsage            int    `yaml:"limit_memory_usage"`
	LimitCpuUsage               int    `yaml:"limit_cpu_usage"`
	SlackUrl                    string `yaml:"slack_url"`
}

func LoadEnv(f string) (map[string]IZONEConfig, error) {
	var envs map[string]IZONEConfig
	data, err := os.ReadFile(f)
	if err != nil {
		return nil, err
	}
	err = yaml.Unmarshal(data, &envs)
	if err != nil {
		return nil, err
	}
	return envs, nil
}
