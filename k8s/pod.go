package k8s

import (
	"context"
	createError "errors"
	"fmt"
	"sync"
	"time"

	"github.com/sjy-dv/IZONE/pkg/slack"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Pod struct {
	Label            string
	Namespace        string
	Logging          bool
	LoggingLevel     int
	ScaleUp          bool
	LimitMemoryUsage int
	LimitCpuUsage    int
	SlackUrl         string
	Interval         time.Duration
	timer            *time.Ticker
	archive          archive
}
type archive struct {
	CPU    int64
	Memory int64
	Status string
}

type podGroup struct {
	mu     sync.Mutex
	groups []*Pod
}

var pods *podGroup

func RegisterPods(pod *Pod) {
	if err := pod.exists(); err != nil {
		if err != nil {
			if errors.IsNotFound(err) {
				warnCh <- fmt.Sprintf("The Pod you registered, %s, does not exist. Please double-check the namespace and name.", pod.Label)
			} else {
				errCh <- err
			}
		}
	} else if err == nil {
		pods.mu.Lock()
		defer pods.mu.Unlock()
		pod.timerstart()
		pods.groups = append(pods.groups, pod)
	}
}

func (p *Pod) exists() error {
	_, err := k8sclient.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Label, metav1.GetOptions{})
	return err
}

func (p *Pod) timerstart() {
	p.timer = time.NewTicker(p.Interval)
	go func() {
		for {
			select {
			case <-p.timer.C:
				if err := p.inspection(); err != nil {
					p.release()
				}
			}
		}
	}()
}

func (p *Pod) release() {
	p.timer.Stop()
	pods.mu.Lock()
	defer pods.mu.Unlock()
	for index, pod := range pods.groups {
		if p == pod {
			pods.groups = append(pods.groups[:index], pods.groups[index+1:]...)
		}
	}
}

func (p *Pod) inspection() error {
	pod, err := k8sclient.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Label, metav1.GetOptions{})
	if err != nil {
		errCh <- fmt.Errorf("The %s pod has failed the check : %v", p.Label, err)
		return err
	}
	phase := string(pod.Status.Phase)
	if phase != "Running" && phase != "Completed" && phase != "Succeeded" {
		slack.Channel <- &slack.IZONEForm{
			Text: func() string {
				if p.archive.Status == "" {
					return fmt.Sprintf("Pod: %s \nThe Status of the Pod `%s` is %s", p.Label, p.Label, phase)
				}
				return fmt.Sprintf("Pod: %s \nThe status change %s -> %s.\nThe Pod `%s` status is not normal.\nPlease check.",
					p.Label, p.archive.Status, phase, p.Label)
			}(),
			Level:      slack.ERROR,
			WebHookUrl: p.SlackUrl,
		}
		return createError.New(abnormallyPods)
	}
	if p.LoggingLevel == 3 {
		p.archive.Status = phase
		return nil
	}
	var curCpu int64
	var curMem int64
	metrics, err := metricsclient.MetricsV1beta1().PodMetricses(p.Namespace).Get(context.TODO(), p.Label, metav1.GetOptions{})
	if err != nil {
		errCh <- fmt.Errorf("Pod: %s \nThe %s pod has failed the check metrics: %v", p.Label, p.Label, err)
		return err
	}
	for _, container := range metrics.Containers {
		curCpu = container.Usage.Cpu().MilliValue()
		curMem = container.Usage.Memory().MilliValue() / MB
	}
	if p.LoggingLevel == 2 {
		if p.archive.Status == "" {
			p.archive.Status = phase
			p.archive.CPU = curCpu
			p.archive.Memory = curMem
			return nil
		} else {
			slack.Channel <- &slack.IZONEForm{
				Text: fmt.Sprintf("Pod: %s \nStatus: %s (%s)\nCpu Usage: %dm -> %dm (%s)\nMemory Usage: %dm -> %dm (%s)",
					p.Label, phase, phaseEqual(phase, p.archive.Status), p.archive.CPU, curCpu, percentage(curCpu, p.archive.CPU),
					p.archive.Memory, curMem, percentage(curMem, p.archive.Memory)),
				Level:      slack.INFO,
				WebHookUrl: p.SlackUrl,
			}
			p.archive.Status = phase
			p.archive.CPU = curCpu
			p.archive.Memory = curMem
			return nil
		}
	}
	// logging_level 1
	if p.archive.Status == "" {
		slack.Channel <- &slack.IZONEForm{
			Text:       fmt.Sprintf("Pod: %s \nStatus: %s\nCpu Usage: %dm\nMemory Usage: %dm", p.Label, phase, curCpu, curMem),
			Level:      slack.INFO,
			WebHookUrl: p.SlackUrl,
		}
		p.archive.Status = phase
		p.archive.CPU = curCpu
		p.archive.Memory = curMem
		return nil
	}
	slack.Channel <- &slack.IZONEForm{
		Text: fmt.Sprintf("Pod: %s \nStatus: %s (%s)\nCpu Usage: %dm -> %dm (%s)\nMemory Usage: %dm -> %dm (%s)",
			p.Label, phase, phaseEqual(phase, p.archive.Status), p.archive.CPU, curCpu, percentage(curCpu, p.archive.CPU),
			p.archive.Memory, curMem, percentage(curMem, p.archive.Memory)),
		Level:      slack.INFO,
		WebHookUrl: p.SlackUrl,
	}
	p.archive.Status = phase
	p.archive.CPU = curCpu
	p.archive.Memory = curMem
	return nil
}
