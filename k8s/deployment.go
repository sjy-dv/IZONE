package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Deployment struct {
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

type deploymentGroup struct {
	mu     sync.Mutex
	groups []*Deployment
}

var deployments *deploymentGroup

func RegisterDeployments(deployment *Deployment) {
	apps, err := deployment.exists()
	if err != nil {
		if errors.IsNotFound(err) {
			warnCh <- fmt.Sprintf("The Deployment you registered, %s, does not exist. Please double-check the namespace and name.", deployment.Label)
			return
		}
		errCh <- err
		return
	}
	deploy, err := deployment.replicas(apps)
	if err != nil {
		errCh <- err
		return
	}
	deployments.mu.Lock()
	defer deployments.mu.Unlock()
	deploy.timestart()
	deployments.groups = append(deployments.groups, deploy)
}

func (d *Deployment) exists() (*appsv1.Deployment, error) {
	deployment, err := k8sclient.AppsV1().Deployments(d.Namespace).Get(context.TODO(), d.Label, metav1.GetOptions{})
	return deployment, err
}

func (d *Deployment) replicas(deployment *appsv1.Deployment) (*Deployment, error) {
	labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicas, err := k8sclient.CoreV1().Pods(d.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	for _, replica := range replicas.Items {
		pod := &Pod{}
		pod.Label = replica.Name
		d.pods = append(d.pods, pod)
	}
	return d, nil
}

func (d *Deployment) timestart() {
	d.timer = time.NewTicker(d.Interval)

}
