package k8s

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
	replicas                    []*sreplica
	timer                       *time.Ticker
}

type sreplica struct {
	Label      string
	identifier int
	archive
}

type serror struct {
	label      string
	identifier int
	err        error
}

type statefulsetGroup struct {
	mu     sync.Mutex
	groups []*Statefulset
}

var statefulsets *statefulsetGroup

func RegisterStatefulsets(statefulset *Statefulset) {
	apps, err := statefulset.exists()
	if err != nil {
		if errors.IsNotFound(err) {
			warnCh <- fmt.Sprintf("The Statefulset you registered, %s, does not exist. Please double-check the namespace and name.", statefulset.Label)
			return
		}
		errCh <- err
		return
	}
	sts, err := statefulset.joinreplicas(apps)
	if err != nil {
		errCh <- err
		return
	}
	statefulsets.mu.Lock()
	defer statefulsets.mu.Unlock()
	sts.timestart()
	statefulsets.groups = append(statefulsets.groups, sts)
}

func (s *Statefulset) exists() (*appsv1.StatefulSet, error) {
	statefulset, err := k8sclient.AppsV1().StatefulSets(s.Namespace).Get(context.TODO(), s.Label, metav1.GetOptions{})
	return statefulset, err
}

func (s *Statefulset) joinreplicas(sts *appsv1.StatefulSet) (*Statefulset, error) {
	labelSelector := metav1.FormatLabelSelector(sts.Spec.Selector)
	replicas, err := k8sclient.CoreV1().Pods(s.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	for _, replica := range replicas.Items {
		pod := &sreplica{}
		pod.Label = replica.Name
		pod.identifier = identifier(replica.Name)
		s.replicas = append(s.replicas, pod)
	}
	return s, nil
}

func identifier(pod string) int {
	label := strings.Split(pod, "-")
	parseLabel, err := strconv.Atoi(label[len(label)-1])
	if err != nil {
		return -1
	}
	return parseLabel
}

func (s *Statefulset) timestart() {
	s.timer = time.NewTicker(s.Interval)
}
