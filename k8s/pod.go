package k8s

import (
	"context"
	"fmt"
	"sync"
	"time"

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
	CPU    int
	Memory int
	Status string
}

type podChannel struct {
	mu       sync.Mutex
	channels []*Pod
}

var pods *podChannel

func RegisterPods(pod *Pod) {
	if err := pod.exists(); err != nil {
		if err != nil {
			if errors.IsNotFound(err) {
				warnCh <- fmt.Sprintf("The Pod you registered, %s, does not exist. Please double-check the namespace and name.", pod.Label)
			}
			errCh <- err
		}
	} else if err == nil {
		pods.mu.Lock()
		defer pods.mu.Unlock()
		pod.timerstart()
		pods.channels = append(pods.channels, pod)
	}
}

func (p *Pod) exists() error {
	_, err := k8sclient.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Label, metav1.GetOptions{})
	if err != nil {
		return err
	}
	return nil
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
	for index, pod := range pods.channels {
		if p == pod {
			pods.channels = append(pods.channels[:index], pods.channels[index+1:]...)
		}
	}
}

func (p *Pod) inspection() error {
	pod, err := k8sclient.CoreV1().Pods(p.Namespace).Get(context.TODO(), p.Label, metav1.GetOptions{})
	if err != nil {
		errCh <- fmt.Errorf("The %s pod has failed the check : %v", p.Label, err)
		return err
	}
	if p.LoggingLevel == 3 {
		if pod.Status.Phase != "Running" && pod.Status.Phase != "Completed" || pod.Status.Phase != "Succeeded" {
			if p.archive.Status == "" {

			}
		}
	}

	return nil
}
