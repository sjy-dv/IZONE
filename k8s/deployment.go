package k8s

import (
	"context"
	createError "errors"
	"fmt"
	"sync"
	"time"

	"github.com/sjy-dv/kslack/pkg/slack"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
	ScaleOut                    bool
	ScaleDownInterval           int
	MinReplicas                 int
	MaxReplicas                 int
	MaxMemoryPercentBeforeScale int
	MaxCpuPercentBeforeScale    int
	replicas                    []*dreplica
	archive                     darchive
	timer                       *time.Ticker
	errCh                       chan derror
}

type dreplica struct {
	Label string
	archive
}

type derror struct {
	label string
	err   error
}

type darchive struct {
	CPU     int64
	Memory  int64
	Replica int
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
	deploy, err := deployment.addreplicas(apps)
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

func (d *Deployment) addreplicas(deployment *appsv1.Deployment) (*Deployment, error) {
	labelSelector := metav1.FormatLabelSelector(deployment.Spec.Selector)
	replicas, err := k8sclient.CoreV1().Pods(d.Namespace).List(context.TODO(), metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, err
	}
	for _, replica := range replicas.Items {
		pod := &dreplica{}
		pod.Label = replica.Name
		d.replicas = append(d.replicas, pod)
	}
	return d, nil
}

func (d *Deployment) timestart() {
	d.timer = time.NewTicker(d.Interval)
	go func() {
		for {
			select {
			case <-d.timer.C:
				if err := d.inspection(); err != nil {
					d.release()
				}
			}
		}
	}()
}

func (d *Deployment) release() {
	d.timer.Stop()
	deployments.mu.Lock()
	defer deployments.mu.Unlock()
	for index, deployment := range deployments.groups {
		if d == deployment {
			deployments.groups = append(deployments.groups[:index], deployments.groups[index+1:]...)
		}
	}
}

func (d *Deployment) inspection() error {
	var (
		totalCpu    int64  = 0
		totalMemory int64  = 0
		slackmsg    string = ""
		adjustment  string = ""
		changesrc   bool   = false
		cr          int    = 0
	)
	d.archive.Replica = len(d.replicas)
	cpuoverflow := 0
	memoverflow := 0
	d.errCh = make(chan derror)
	for _, replica := range d.replicas {
		if err := replica.replicaInspection(d.Namespace, d.LoggingLevel); err != nil {
			d.errCh <- derror{
				label: replica.Label,
				err:   err,
			}
		}
		if d.LimitCpuUsage < int(replica.CPU) {
			cpuoverflow++
		}
		if d.LimitMemoryUsage < int(replica.Memory) {
			memoverflow++
		}
		totalCpu += replica.CPU
		totalMemory += replica.Memory
	}
	/*
		if replica 3
		over resource replica 2
		3-1 <= 2 -> true
		3-1 <= 1 -> false || want replica 2 but mistake deploy 1
		if true increase replica on spec
	*/
	if len(d.replicas)-1 <= cpuoverflow || len(d.replicas)-1 <= memoverflow ||
		len(d.replicas) <= d.MinReplicas {
		if len(d.replicas) < d.MaxReplicas {
			if err := d.increaseReplica(); err != nil {
				errCh <- err
				adjustment = fmt.Sprintf("Replica Increase Failed: %v", err)
			} else if err == nil {
				changesrc = true
				d.archive.Replica++
				cr = 1
			}
		}
	}
	/*
		if replica 3
		want maintain replica 2
		over resource 0
		one try decrease one by one
		if 4 4-1 , 3 next 3 -> 2 ok
	*/
	if cpuoverflow == 0 && memoverflow == 0 {
		if len(d.replicas) > d.MinReplicas {
			if err := d.decreaseReplica(); err != nil {
				errCh <- err
				adjustment = fmt.Sprintf("Replica Decrease Failed: %v", err)
			} else if err == nil {
				changesrc = true
				d.archive.Replica--
				cr = -1
			}
		}
	}
	close(d.errCh)
	if d.LoggingLevel == 3 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.KSlackForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Deployment: %s%s", d.Label, slackmsg)
					}
					return fmt.Sprintf("Deployment: %s\n%s%s", d.Label, adjustment, slackmsg)
				}(),
				Level:      slack.ERROR,
				WebHookUrl: d.SlackUrl,
			}
		}
		return nil
	}
	/*
		add replcas, cpu, mem init 0 -> 1
	*/
	if totalCpu == 0 {

	}
	if d.LoggingLevel == 2 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.KSlackForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d%s",
							d.Label, totalCpu, totalMemory, slackmsg)
					}
					return fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
						d.Label, totalCpu, totalMemory, adjustment, slackmsg)
				}(),
				Level:      slack.WARNING,
				WebHookUrl: d.SlackUrl,
			}
		} else {
			// non-error
			slack.Channel <- &slack.KSlackForm{
				Text: func() string {
					base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
					if changesrc {
						return fmt.Sprintf(
							"%s\n%s", base, func() string {
								if cr == 1 {
									return fmt.Sprintf(
										"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
										d.archive.Replica-1, d.archive.Replica, cpuoverflow, memoverflow)
								}
								return fmt.Sprintf("decrease replicas %d -> %d", d.archive.Replica+1, d.archive.Replica)
							}())
					}
					return base
				}(),
				Level:      slack.INFO,
				WebHookUrl: d.SlackUrl,
			}
		}
		return nil
	}
	for err := range d.errCh {
		slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
	}
	if slackmsg != "" {
		// error scenario
		slack.Channel <- &slack.KSlackForm{
			Text: func() string {
				if adjustment == "" {
					return fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d%s",
						d.Label, totalCpu, totalMemory, slackmsg)
				}
				return fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
					d.Label, totalCpu, totalMemory, adjustment, slackmsg)
			}(),
			Level:      slack.WARNING,
			WebHookUrl: d.SlackUrl,
		}
	}
	slack.Channel <- &slack.KSlackForm{
		Text: func() string {
			base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
			if changesrc {
				return fmt.Sprintf(
					"%s\n%s", base, func() string {
						if cr == 1 {
							return fmt.Sprintf(
								"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
								d.archive.Replica-1, d.archive.Replica, cpuoverflow, memoverflow)
						}
						return fmt.Sprintf("decrease replicas %d -> %d", d.archive.Replica+1, d.archive.Replica)
					}())
			}
			return base
		}(),
		Level:      slack.INFO,
		WebHookUrl: d.SlackUrl,
	}
	return nil
}

func (d *dreplica) replicaInspection(namespace string, lv int) error {
	pod, err := k8sclient.CoreV1().Pods(namespace).Get(context.TODO(), d.Label, metav1.GetOptions{})
	if err != nil {
		errCh <- fmt.Errorf("The %s pod has failed the check : %v", d.Label, err)
		return err
	}
	phase := string(pod.Status.Phase)
	if phase != "Running" && phase != "Completed" && phase != "Succeeded" {
		return createError.New(abnormallyPods)
	}
	if lv == 3 {
		return nil
	}
	metrics, err := metricsclient.MetricsV1beta1().PodMetricses(namespace).Get(context.TODO(), d.Label, metav1.GetOptions{})
	if err != nil {
		errCh <- fmt.Errorf("The %s pod has failed the check metrics: %v", d.Label, err)
		return err
	}
	d.Status = phase
	for _, container := range metrics.Containers {
		d.CPU = container.Usage.Cpu().MilliValue()
		d.Memory = container.Usage.Cpu().MilliValue() / MB
	}
	return nil
}

func (d *Deployment) increaseReplica() error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := k8sclient.AppsV1().Deployments(d.Namespace).
			Get(context.TODO(), d.Label, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if *deployment.Spec.Replicas < int32(10) {
			*deployment.Spec.Replicas++
		} else {
			return createError.New("Max replicas reached")
		}
		_, err = k8sclient.AppsV1().Deployments(d.Namespace).
			Update(context.TODO(), deployment, metav1.UpdateOptions{})
		return err
	})
}

func (d *Deployment) decreaseReplica() error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		deployment, err := k8sclient.AppsV1().Deployments(d.Namespace).
			Get(context.TODO(), d.Label, metav1.GetOptions{})
		if err != nil {
			return err
		}
		*deployment.Spec.Replicas--
		_, err = k8sclient.AppsV1().Deployments(d.Namespace).
			Update(context.TODO(), deployment, metav1.UpdateOptions{})
		return err
	})
}
