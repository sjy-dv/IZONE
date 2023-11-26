package k8s

import (
	"context"
	createError "errors"
	"fmt"
	"sync"
	"time"

	"github.com/sjy-dv/IZONE/pkg/slack"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
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
	//scale-out
	Mediate Mediate
	//scale-up
	CpuMediate    Mediate
	MemoryMediate Mediate
}

type Mediate time.Time

func (m Mediate) cycle(term int) bool {
	return time.Since(time.Time(m)).Seconds() >= float64(term)
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
	deploy, err := deployment.joinreplicas(apps)
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

func (d *Deployment) joinreplicas(deployment *appsv1.Deployment) (*Deployment, error) {
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
				switch d.checkScaleTodo() {
				case NORMAL:
					if err := d.inspection(); err != nil {
						if err.Error() == "clear" {
							d.release()
						}
						d.refresh()
					}
					break
				case SCALE_OUT:
					if err := d.scaleoutinspection(); err != nil {
						if err.Error() == "clear" {
							d.release()
						}
						d.refresh()
					}
					break
				case SCALE_UP:
					if err := d.scaleupinspection(); err != nil {
						if err.Error() == "clear" {
							d.release()
						}
						d.refresh()
					}
					break
				}

			}
		}
	}()
}

func (d *Deployment) refresh() {
	deployments.mu.Lock()
	defer deployments.mu.Unlock()
	deployment, err := k8sclient.AppsV1().Deployments(d.Namespace).Get(context.TODO(), d.Label, metav1.GetOptions{})
	if err != nil {
		return
	}
	rd, err := d.joinreplicas(deployment)
	if err != nil {
		return
	}
	d.replicas = rd.replicas
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
	_, err := d.exists()
	if err != nil {
		return createError.New("clear")
	}
	var (
		totalCpu    int64  = 0
		totalMemory int64  = 0
		slackmsg    string = ""
	)
	d.archive.Replica = len(d.replicas)
	d.errCh = make(chan derror)
	for _, replica := range d.replicas {
		if err := replica.replicaInspection(d.Namespace, d.LoggingLevel); err != nil {
			d.errCh <- derror{
				label: replica.Label,
				err:   err,
			}
		}
		totalCpu += replica.CPU
		totalMemory += replica.Memory
	}
	close(d.errCh)
	if d.LoggingLevel == 3 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
				Text:       fmt.Sprintf("Deployment: %s%s", d.Label, slackmsg),
				Level:      slack.ERROR,
				WebHookUrl: d.SlackUrl,
			}
		}
	}
	/*
		add replcas, cpu, mem init 0 -> 1
	*/
	if totalCpu == 0 {
		totalCpu = 1
	}
	if totalMemory == 0 {
		totalMemory = 1
	}
	d.archive.CPU = totalCpu
	d.archive.Memory = totalMemory
	if d.LoggingLevel == 2 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
				Text: fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d%s",
					d.Label, totalCpu, totalMemory, slackmsg),
				Level:      slack.WARNING,
				WebHookUrl: d.SlackUrl,
			}
		} else {
			// non-error
			slack.Channel <- &slack.IZONEForm{
				Text:       fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory),
				Level:      slack.INFO,
				WebHookUrl: d.SlackUrl,
			}
		}
	}
	for err := range d.errCh {
		slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
	}
	if slackmsg != "" {
		// error scenario
		slack.Channel <- &slack.IZONEForm{
			Text: fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d%s",
				d.Label, totalCpu, totalMemory, slackmsg),
			Level:      slack.WARNING,
			WebHookUrl: d.SlackUrl,
		}
	} else {
		slack.Channel <- &slack.IZONEForm{
			Text:       fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory),
			Level:      slack.INFO,
			WebHookUrl: d.SlackUrl,
		}
	}
	return nil
}

func (d *Deployment) scaleoutinspection() error {

	deployments, err := d.exists()
	if err != nil {
		return createError.New("clear")
	}

	var (
		totalCpu         int64  = 0
		totalMemory      int64  = 0
		slackmsg         string = ""
		adjustment       string = ""
		changesrc        bool   = false
		cpuoverflow      int    = 0
		memoverflow      int    = 0
		preCommitReplica int    = 0
	)
	preCommitReplica = d.archive.Replica
	d.archive.Replica = int(*deployments.Spec.Replicas)

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
				d.archive.Mediate = Mediate(time.Now())
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
			if d.archive.Mediate.cycle(d.ScaleDownInterval) {
				if err := d.decreaseReplica(); err != nil {
					errCh <- err
					adjustment = fmt.Sprintf("Replica Decrease Failed: %v", err)
				} else if err == nil {
					changesrc = true
					d.archive.Replica--
				}
			}
		}
	}
	close(d.errCh)
	if d.LoggingLevel == 3 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
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
	}
	/*
		add replcas, cpu, mem init 0 -> 1
	*/

	if d.LoggingLevel == 2 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
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
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
					if changesrc {
						return fmt.Sprintf(
							"%s\n%s", base, func() string {
								if preCommitReplica < d.archive.Replica {
									return fmt.Sprintf(
										"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
										preCommitReplica, d.archive.Replica, cpuoverflow, memoverflow)
								} else if preCommitReplica > d.archive.Replica {
									return fmt.Sprintf("decrease replicas %d -> %d", preCommitReplica, d.archive.Replica)
								}
								return "The configuration of the replica has been changed."
							}())
					}
					return base
				}(),
				Level:      slack.INFO,
				WebHookUrl: d.SlackUrl,
			}
		}
	}
	for err := range d.errCh {
		slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
	}
	if slackmsg != "" {
		// error scenario
		slack.Channel <- &slack.IZONEForm{
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
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
				if changesrc {
					return fmt.Sprintf(
						"%s\n%s", base, func() string {
							if preCommitReplica < d.archive.Replica {
								return fmt.Sprintf(
									"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
									preCommitReplica, d.archive.Replica, cpuoverflow, memoverflow)
							} else if preCommitReplica > d.archive.Replica {
								return fmt.Sprintf("decrease replicas %d -> %d", preCommitReplica, d.archive.Replica)
							}
							return "The configuration of the replica has been changed."
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

func (d *Deployment) checkScaleTodo() int {
	if d.ScaleOut {
		return SCALE_OUT
	}
	if d.ScaleUp {
		return SCALE_UP
	}
	return NORMAL
}

/*
Scale-up is typically suitable for scenarios with a single Pod,
such as a single-instance MySQL.
When multiple replicas are declared and scale-up is performed,
if one replica fits the scenario,
the resource allocation for all replicas associated with the deployment automatically increases.
*/
func (d *Deployment) scaleupinspection() error {
	deployments, err := d.exists()
	if err != nil {
		return createError.New("clear")
	}
	var (
		slackmsg    string = ""
		changesrc   bool   = false
		adjustment  string = ""
		curCpu      int64  = 0
		curMemory   int64  = 0
		totalCpu    int64  = 0
		totalMemory int64  = 0
	)
	d.errCh = make(chan derror)
	for _, replica := range d.replicas {
		if err := replica.replicaInspection(d.Namespace, d.LoggingLevel); err != nil {
			d.errCh <- derror{
				label: replica.Label,
				err:   err,
			}
		}
		if curCpu > replica.CPU {
			curCpu = replica.CPU
		}
		if curMemory > replica.Memory {
			curMemory = replica.Memory
		}
		totalCpu += replica.CPU
		totalMemory += replica.Memory
	}
	rememberCpu := d.archive.CPU
	rememberMem := d.archive.Memory
	//If there are no requests, scale-up cannot be performed
	// increase 20% current resouces
	if curCpu > int64(d.LimitCpuUsage) || curMemory > int64(d.LimitMemoryUsage) {
		// if resources nil value. init apply -f yaml. assigned empty resource
		if deployments.Spec.Template.Spec.Containers[0].Resources.Requests == nil {
			deployments.Spec.Template.Spec.Containers[0].Resources.Requests = v1.ResourceList{}
		}
		if deployments.Spec.Template.Spec.Containers[0].Resources.Limits == nil {
			deployments.Spec.Template.Spec.Containers[0].Resources.Limits = v1.ResourceList{}
		}
		cpuReq, cpuLim := resource.MustParse(fmt.Sprintf("%dm", int64(float64(curCpu)*1.2))), resource.MustParse(fmt.Sprintf("%dm", int64(float64(curCpu)*1.2)))
		memReq, memLim := resource.MustParse(fmt.Sprintf("%dMi", int64(float64(curMemory)*1.2))), resource.MustParse(fmt.Sprintf("%dMi", int64(float64(curMemory)*1.2)))
		if curCpu > int64(d.LimitCpuUsage) {
			//only cpu update
			deployments.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = cpuReq
			deployments.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = cpuLim
			d.LimitCpuUsage = int(curCpu)
		}
		if curMemory > int64(d.LimitMemoryUsage) {
			//only memory update
			deployments.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory] = memReq
			deployments.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = memLim
			d.LimitMemoryUsage = int(curMemory)
		}
		if curCpu > int64(d.LimitCpuUsage) && curMemory > int64(d.LimitMemoryUsage) {
			// update all
			deployments.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = cpuReq
			deployments.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = cpuLim
			deployments.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory] = memReq
			deployments.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = memLim
			d.LimitCpuUsage = int(curCpu)
			d.LimitMemoryUsage = int(curMemory)
		}
		if err := d.resourcesApply(deployments); err != nil {
			errCh <- err
			adjustment = fmt.Sprintf("Failed to redefine the resources : %v", err)
		} else if err == nil {
			changesrc = true
		}
	}
	close(d.errCh)
	if d.LoggingLevel == 3 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
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
	}
	if d.LoggingLevel == 2 {
		for err := range d.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
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
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
					if changesrc {
						return fmt.Sprintf(
							"%s\n%s", base, func() string {
								if curCpu > rememberCpu {
									return fmt.Sprintf("Redefine Resouce Cpu: %vm (request, limit)", d.LimitCpuUsage)
								}
								if curMemory > rememberMem {
									return fmt.Sprintf("Redefine Resource Memory: %vm (request, limit)", d.LimitMemoryUsage)
								}
								return fmt.Sprintf("Redefine Resources Group\nCpu: %vm (request, limit)\nMemory: %vm (request, limit)",
									d.LimitCpuUsage, d.LimitMemoryUsage)
							}())
					}
					return base
				}(),
				Level:      slack.INFO,
				WebHookUrl: d.SlackUrl,
			}
		}
	}
	if slackmsg != "" {
		slack.Channel <- &slack.IZONEForm{
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
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				base := fmt.Sprintf("Deployment: %s\nCpu Usage: %d\nMemory Usage: %d", d.Label, totalCpu, totalMemory)
				if changesrc {
					return fmt.Sprintf(
						"%s\n%s", base, func() string {
							if curCpu > rememberCpu {
								return fmt.Sprintf("Redefine Resouce Cpu: %vm (request, limit)", d.LimitCpuUsage)
							}
							if curMemory > rememberMem {
								return fmt.Sprintf("Redefine Resource Memory: %vm (request, limit)", d.LimitMemoryUsage)
							}
							return fmt.Sprintf("Redefine Resources Group\nCpu: %vm (request, limit)\nMemory: %vm (request, limit)",
								d.LimitCpuUsage, d.LimitMemoryUsage)
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

func (d *Deployment) resourcesApply(apply *appsv1.Deployment) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := k8sclient.AppsV1().Deployments(d.Namespace).
			Update(context.TODO(), apply, metav1.UpdateOptions{})
		return err
	})
}
