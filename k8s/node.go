package k8s

import (
	"context"
	"fmt"
	"time"

	"github.com/sjy-dv/IZONE/internal/channel"
	"github.com/sjy-dv/IZONE/pkg/slack"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type Node struct {
	Interval time.Duration
	SlackUrl string
	timer    *time.Ticker
}

func RegisterNode(node *Node) {
	node.watch()
}

func (n *Node) watch() {
	n.timer = time.NewTicker(n.Interval)
	go func() {
		for {
			select {
			case <-n.timer.C:
				var headerMsg string
				var bodyMsg string
				var footMsg string
				nodes, err := k8sclient.CoreV1().Nodes().
					List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					channel.ErrCh <- err
					nodes = nil
				}
				if nodes != nil {
					for index, node := range nodes.Items {
						if index == 0 {
							headerMsg = fmt.Sprintf("NODE LIST\n%s-%s", node.Name, node.UID)
						}
						headerMsg = fmt.Sprintf("%s\n%s-%s", headerMsg, node.Name, node.UID)
					}
				}
				metrics, err := metricsclient.MetricsV1beta1().NodeMetricses().
					List(context.TODO(), metav1.ListOptions{})
				if err != nil {
					channel.ErrCh <- err
					metrics = nil
				}
				if metrics != nil {
					for index, metric := range metrics.Items {
						if index == 0 {
							bodyMsg = fmt.Sprintf("NODE RESOURCE USAGE\n%s\t%d(Cpu-Milicore)\t%d(Memory-MiB)",
								metric.Name, metric.Usage.Cpu().MilliValue(), metric.Usage.Memory().MilliValue()/MB)
						}
						bodyMsg = fmt.Sprintf("%s\n%s\t%d(Cpu-Milicore)\t%d(Memory-MiB)",
							bodyMsg, metric.Name, metric.Usage.Cpu().MilliValue(), metric.Usage.Memory().MilliValue()/MB)
					}
				}
				events, err := k8sclient.CoreV1().Events(metav1.NamespaceAll).
					List(context.TODO(), metav1.ListOptions{
						FieldSelector: "involvedObject.kind=Node",
					})
				if err != nil {
					channel.ErrCh <- err
					events = nil
				}
				if events != nil {
					for index, event := range events.Items {
						if index == 0 {
							footMsg = fmt.Sprintf("NODE EVENTS\nNode: %s, Type: %s, Reason: %s, Message: %s",
								event.InvolvedObject.Name, event.Type, event.Reason, event.Message)
						}
						footMsg = fmt.Sprintf("%s\nNode: %s, Type: %s, Reason: %s, Message: %s",
							footMsg, event.InvolvedObject.Name, event.Type, event.Reason, event.Message)
					}
				}
				if headerMsg == "" {
					headerMsg = "NODE IS NOT READY"
				}
				if bodyMsg == "" {
					bodyMsg = "NODE IS NOT READY"
				}
				slack.Channel <- &slack.IZONEForm{
					Text:       fmt.Sprintf("%s\n%s\n%s", headerMsg, bodyMsg, footMsg),
					Level:      slack.INFO,
					WebHookUrl: n.SlackUrl,
				}
			}
		}
	}()
}
