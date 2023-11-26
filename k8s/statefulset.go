package k8s

import (
	"context"
	createError "errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/sjy-dv/IZONE/internal/channel"
	"github.com/sjy-dv/IZONE/pkg/slack"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/util/retry"
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
	archive                     darchive
	errCh                       chan serror
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
			channel.WarnCh <- fmt.Sprintf("The Statefulset you registered, %s, does not exist. Please double-check the namespace and name.", statefulset.Label)
			return
		}
		channel.ErrCh <- err
		return
	}
	sts, err := statefulset.joinreplicas(apps)
	if err != nil {
		channel.ErrCh <- err
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
	go func() {
		for {
			select {
			case <-s.timer.C:
				switch s.checkScaleTodo() {
				case NORMAL:
					if err := s.inspection(); err != nil {
						if err.Error() == "clear" {
							s.release()
						}
						s.refresh()
					}
					break
				case SCALE_OUT:
					if err := s.scaleoutinspection(); err != nil {
						if err.Error() == "clear" {
							s.release()
						}
						s.refresh()
					}
					break
				case SCALE_UP:
					if err := s.scaleupinspection(); err != nil {
						if err.Error() == "clear" {
							s.release()
						}
						s.refresh()
					}
					break
				}

			}
		}
	}()
}

func (s *Statefulset) refresh() {
	statefulsets.mu.Lock()
	defer statefulsets.mu.Unlock()
	sts, err := k8sclient.AppsV1().StatefulSets(s.Namespace).Get(context.TODO(), s.Label, metav1.GetOptions{})
	if err != nil {
		return
	}
	rd, err := s.joinreplicas(sts)
	if err != nil {
		return
	}
	s.replicas = rd.replicas
}

func (s *Statefulset) release() {
	s.timer.Stop()
	statefulsets.mu.Lock()
	defer statefulsets.mu.Unlock()
	for index, statfulset := range statefulsets.groups {
		if s == statfulset {
			statefulsets.groups = append(statefulsets.groups[:index], statefulsets.groups[index+1:]...)
		}
	}
}

func (s *Statefulset) inspection() error {
	_, err := s.exists()
	if err != nil {
		return createError.New("clear")
	}
	var (
		totalCpu    int64  = 0
		totalMemory int64  = 0
		slackmsg    string = ""
	)
	s.archive.Replica = len(s.replicas)
	s.errCh = make(chan serror)
	for _, replica := range s.replicas {
		if err := replica.replicaInspection(s.Namespace, s.LoggingLevel); err != nil {
			s.errCh <- serror{
				label:      replica.Label,
				identifier: identifier(replica.Label),
				err:        err,
			}
		}
		totalCpu += replica.CPU
		totalMemory += replica.Memory
	}
	close(s.errCh)
	if s.LoggingLevel == 3 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
				Text:       fmt.Sprintf("Statefulset: %s%s", s.Label, slackmsg),
				Level:      slack.ERROR,
				WebHookUrl: s.SlackUrl,
			}
		}
	}

	s.archive.CPU = totalCpu
	s.archive.Memory = totalMemory
	if s.LoggingLevel == 2 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
				Text: fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
					s.Label, totalCpu, totalMemory, slackmsg),
				Level:      slack.WARNING,
				WebHookUrl: s.SlackUrl,
			}
		} else {
			// non-error
			slack.Channel <- &slack.IZONEForm{
				Text:       fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory),
				Level:      slack.INFO,
				WebHookUrl: s.SlackUrl,
			}
		}
	}
	for err := range s.errCh {
		slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
	}
	if slackmsg != "" {
		// error scenario
		slack.Channel <- &slack.IZONEForm{
			Text: fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
				s.Label, totalCpu, totalMemory, slackmsg),
			Level:      slack.WARNING,
			WebHookUrl: s.SlackUrl,
		}
	} else {
		slack.Channel <- &slack.IZONEForm{
			Text:       fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory),
			Level:      slack.INFO,
			WebHookUrl: s.SlackUrl,
		}
	}
	return nil
}

func (s *Statefulset) scaleoutinspection() error {

	statefulset, err := s.exists()
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
	preCommitReplica = s.archive.Replica
	s.archive.Replica = int(*statefulset.Spec.Replicas)

	s.errCh = make(chan serror)
	for _, replica := range s.replicas {
		if err := replica.replicaInspection(s.Namespace, s.LoggingLevel); err != nil {
			s.errCh <- serror{
				label: replica.Label,
				err:   err,
			}
		}
		if s.LimitCpuUsage < int(replica.CPU) {
			cpuoverflow++
		}
		if s.LimitMemoryUsage < int(replica.Memory) {
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
	if len(s.replicas)-1 <= cpuoverflow || len(s.replicas)-1 <= memoverflow ||
		len(s.replicas) <= s.MinReplicas {
		if len(s.replicas) < s.MaxReplicas {
			if err := s.increaseReplica(); err != nil {
				channel.ErrCh <- err
				adjustment = fmt.Sprintf("Replica Increase Failed: %v", err)
			} else if err == nil {
				changesrc = true
				s.archive.Replica++
				s.archive.Mediate = Mediate(time.Now())
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
		if len(s.replicas) > s.MinReplicas {
			if s.archive.Mediate.cycle(s.ScaleDownInterval) {
				if err := s.decreaseReplica(); err != nil {
					channel.ErrCh <- err
					adjustment = fmt.Sprintf("Replica Decrease Failed: %v", err)
				} else if err == nil {
					changesrc = true
					s.archive.Replica--
				}
			}
		}
	}
	close(s.errCh)
	if s.LoggingLevel == 3 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Statefulset: %s%s", s.Label, slackmsg)
					}
					return fmt.Sprintf("Statefulset: %s\n%s%s", s.Label, adjustment, slackmsg)
				}(),
				Level:      slack.ERROR,
				WebHookUrl: s.SlackUrl,
			}
		}
	}
	/*
		add replcas, cpu, mem init 0 -> 1
	*/

	if s.LoggingLevel == 2 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
							s.Label, totalCpu, totalMemory, slackmsg)
					}
					return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
						s.Label, totalCpu, totalMemory, adjustment, slackmsg)
				}(),
				Level:      slack.WARNING,
				WebHookUrl: s.SlackUrl,
			}
		} else {
			// non-error
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					base := fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory)
					if changesrc {
						return fmt.Sprintf(
							"%s\n%s", base, func() string {
								if preCommitReplica < s.archive.Replica {
									return fmt.Sprintf(
										"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
										preCommitReplica, s.archive.Replica, cpuoverflow, memoverflow)
								} else if preCommitReplica > s.archive.Replica {
									return fmt.Sprintf("decrease replicas %d -> %d", preCommitReplica, s.archive.Replica)
								}
								return "The configuration of the replica has been changes."
							}())
					}
					return base
				}(),
				Level:      slack.INFO,
				WebHookUrl: s.SlackUrl,
			}
		}
	}
	for err := range s.errCh {
		slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
	}
	if slackmsg != "" {
		// error scenario
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				if adjustment == "" {
					return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
						s.Label, totalCpu, totalMemory, slackmsg)
				}
				return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
					s.Label, totalCpu, totalMemory, adjustment, slackmsg)
			}(),
			Level:      slack.WARNING,
			WebHookUrl: s.SlackUrl,
		}
	} else {
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				base := fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory)
				if changesrc {
					return fmt.Sprintf(
						"%s\n%s", base, func() string {
							if preCommitReplica < s.archive.Replica {
								return fmt.Sprintf(
									"increase replicas %d -> %d (over cpu replicas %d, over memory replicas %d)",
									preCommitReplica, s.archive.Replica, cpuoverflow, memoverflow)
							} else if preCommitReplica > s.archive.Replica {
								return fmt.Sprintf("decrease replicas %d -> %d", preCommitReplica, s.archive.Replica)
							}
							return "The configuration of the replica has been changes."
						}())
				}
				return base
			}(),
			Level:      slack.INFO,
			WebHookUrl: s.SlackUrl,
		}
	}
	return nil
}

func (s *sreplica) replicaInspection(namespace string, lv int) error {
	pod, err := k8sclient.CoreV1().Pods(namespace).Get(context.TODO(), s.Label, metav1.GetOptions{})
	if err != nil {
		channel.ErrCh <- fmt.Errorf("The %s pod has failed the check : %v", s.Label, err)
		return err
	}
	phase := string(pod.Status.Phase)
	if phase != "Running" && phase != "Completed" && phase != "Succeeded" {
		return createError.New(abnormallyPods)
	}
	if lv == 3 {
		return nil
	}
	metrics, err := metricsclient.MetricsV1beta1().PodMetricses(namespace).Get(context.TODO(), s.Label, metav1.GetOptions{})
	if err != nil {
		channel.ErrCh <- fmt.Errorf("The %s pod has failed the check metrics: %v", s.Label, err)
		return err
	}
	s.Status = phase
	for _, container := range metrics.Containers {
		s.CPU = container.Usage.Cpu().MilliValue()
		s.Memory = container.Usage.Memory().MilliValue() / MB
	}
	return nil
}

func (s *Statefulset) increaseReplica() error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		statefulset, err := k8sclient.AppsV1().StatefulSets(s.Namespace).
			Get(context.TODO(), s.Label, metav1.GetOptions{})
		if err != nil {
			return err
		}
		if *statefulset.Spec.Replicas < int32(10) {
			*statefulset.Spec.Replicas++
		} else {
			return createError.New("Max replicas reached")
		}
		_, err = k8sclient.AppsV1().StatefulSets(s.Namespace).
			Update(context.TODO(), statefulset, metav1.UpdateOptions{})
		return err
	})
}

func (s *Statefulset) decreaseReplica() error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		statefulset, err := k8sclient.AppsV1().StatefulSets(s.Namespace).
			Get(context.TODO(), s.Label, metav1.GetOptions{})
		if err != nil {
			return err
		}
		*statefulset.Spec.Replicas--
		_, err = k8sclient.AppsV1().StatefulSets(s.Namespace).
			Update(context.TODO(), statefulset, metav1.UpdateOptions{})
		return err
	})
}

func (s *Statefulset) checkScaleTodo() int {
	if s.ScaleOut {
		return SCALE_OUT
	}
	if s.ScaleUp {
		return SCALE_UP
	}
	return NORMAL
}

/*
Scale-up is typically suitable for scenarios with a single Pod,
such as a single-instance MySQL.
When multiple replicas are declared and scale-up is performed,
if one replica fits the scenario,
the resource allocation for all replicas associated with the statefulset automatically increases.
*/
func (s *Statefulset) scaleupinspection() error {
	statefulset, err := s.exists()
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
	s.errCh = make(chan serror)
	for _, replica := range s.replicas {
		if err := replica.replicaInspection(s.Namespace, s.LoggingLevel); err != nil {
			s.errCh <- serror{
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
	rememberCpu := s.archive.CPU
	rememberMem := s.archive.Memory
	//If there are no requests, scale-up cannot be performed
	// increase 20% current resouces
	if curCpu > int64(s.LimitCpuUsage) || curMemory > int64(s.LimitMemoryUsage) {
		// if resources nil value. init apply -f yaml. assigned empty resource
		if statefulset.Spec.Template.Spec.Containers[0].Resources.Requests == nil {
			statefulset.Spec.Template.Spec.Containers[0].Resources.Requests = v1.ResourceList{}
		}
		if statefulset.Spec.Template.Spec.Containers[0].Resources.Limits == nil {
			statefulset.Spec.Template.Spec.Containers[0].Resources.Limits = v1.ResourceList{}
		}
		cpuReq, cpuLim := resource.MustParse(fmt.Sprintf("%dm", int64(float64(curCpu)*1.2))), resource.MustParse(fmt.Sprintf("%dm", int64(float64(curCpu)*1.2)))
		memReq, memLim := resource.MustParse(fmt.Sprintf("%dMi", int64(float64(curMemory)*1.2))), resource.MustParse(fmt.Sprintf("%dMi", int64(float64(curMemory)*1.2)))
		if curCpu > int64(s.LimitCpuUsage) {
			//only cpu update
			statefulset.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = cpuReq
			statefulset.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = cpuLim
			s.LimitCpuUsage = int(curCpu)
		}
		if curMemory > int64(s.LimitMemoryUsage) {
			//only memory update
			statefulset.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory] = memReq
			statefulset.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = memLim
			s.LimitMemoryUsage = int(curMemory)
		}
		if curCpu > int64(s.LimitCpuUsage) && curMemory > int64(s.LimitMemoryUsage) {
			// update all
			statefulset.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceCPU] = cpuReq
			statefulset.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceCPU] = cpuLim
			statefulset.Spec.Template.Spec.Containers[0].Resources.Requests[v1.ResourceMemory] = memReq
			statefulset.Spec.Template.Spec.Containers[0].Resources.Limits[v1.ResourceMemory] = memLim
			s.LimitCpuUsage = int(curCpu)
			s.LimitMemoryUsage = int(curMemory)
		}
		if err := s.resourcesApply(statefulset); err != nil {
			channel.ErrCh <- err
			adjustment = fmt.Sprintf("Failed to redefine the resources : %v", err)
		} else if err == nil {
			changesrc = true
		}
	}
	close(s.errCh)
	if s.LoggingLevel == 3 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Statefulset: %s%s", s.Label, slackmsg)
					}
					return fmt.Sprintf("Statefulset: %s\n%s%s", s.Label, adjustment, slackmsg)
				}(),
				Level:      slack.ERROR,
				WebHookUrl: s.SlackUrl,
			}
		}
	}
	if s.LoggingLevel == 2 {
		for err := range s.errCh {
			slackmsg = fmt.Sprintf("%s\n[ERROR] %s is status abnormal", slackmsg, err.label)
		}
		if slackmsg != "" {
			// error scenario
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					if adjustment == "" {
						return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
							s.Label, totalCpu, totalMemory, slackmsg)
					}
					return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
						s.Label, totalCpu, totalMemory, adjustment, slackmsg)
				}(),
				Level:      slack.WARNING,
				WebHookUrl: s.SlackUrl,
			}
		} else {
			slack.Channel <- &slack.IZONEForm{
				Text: func() string {
					base := fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory)
					if changesrc {
						return fmt.Sprintf(
							"%s\n%s", base, func() string {
								if curCpu > rememberCpu {
									return fmt.Sprintf("Redefine Resouce Cpu: %vm (request, limit)", s.LimitCpuUsage)
								}
								if curMemory > rememberMem {
									return fmt.Sprintf("Redefine Resource Memory: %vm (request, limit)", s.LimitMemoryUsage)
								}
								return fmt.Sprintf("Redefine Resources Group\nCpu: %vm (request, limit)\nMemory: %vm (request, limit)",
									s.LimitCpuUsage, s.LimitMemoryUsage)
							}())
					}
					return base
				}(),
				Level:      slack.INFO,
				WebHookUrl: s.SlackUrl,
			}
		}
	}
	if slackmsg != "" {
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				if adjustment == "" {
					return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d%s",
						s.Label, totalCpu, totalMemory, slackmsg)
				}
				return fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d\n%s%s",
					s.Label, totalCpu, totalMemory, adjustment, slackmsg)
			}(),
			Level:      slack.WARNING,
			WebHookUrl: s.SlackUrl,
		}
	} else {
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				base := fmt.Sprintf("Statefulset: %s\nCpu Usage: %d\nMemory Usage: %d", s.Label, totalCpu, totalMemory)
				if changesrc {
					return fmt.Sprintf(
						"%s\n%s", base, func() string {
							if curCpu > rememberCpu {
								return fmt.Sprintf("Redefine Resouce Cpu: %vm (request, limit)", s.LimitCpuUsage)
							}
							if curMemory > rememberMem {
								return fmt.Sprintf("Redefine Resource Memory: %vm (request, limit)", s.LimitMemoryUsage)
							}
							return fmt.Sprintf("Redefine Resources Group\nCpu: %vm (request, limit)\nMemory: %vm (request, limit)",
								s.LimitCpuUsage, s.LimitMemoryUsage)
						}())
				}
				return base
			}(),
			Level:      slack.INFO,
			WebHookUrl: s.SlackUrl,
		}
	}
	return nil
}

func (s *Statefulset) resourcesApply(apply *appsv1.StatefulSet) error {
	return retry.RetryOnConflict(retry.DefaultRetry, func() error {
		_, err := k8sclient.AppsV1().StatefulSets(s.Namespace).
			Update(context.TODO(), apply, metav1.UpdateOptions{})
		return err
	})
}
