package k8s

import (
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	metrics "k8s.io/metrics/pkg/client/clientset/versioned"
)

var k8sclient *kubernetes.Clientset
var metricsclient *metrics.Clientset

func ConfigK8s(os string) error {
	loadGroups()
	var k8sCfg *rest.Config
	if os == "windows" {
		var kubeconfig string
		if home := homedir.HomeDir(); home != "" {
			kubeconfig = filepath.Join(home, ".kube", "config")
		}
		config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			return err
		}
		k8sCfg = config
	} else {
		config, err := rest.InClusterConfig()
		if err != nil {
			return err
		}
		k8sCfg = config
	}
	clientset, err := kubernetes.NewForConfig(k8sCfg)
	if err != nil {
		return err
	}
	metricsclientset, err := metrics.NewForConfig(k8sCfg)
	if err != nil {
		return err
	}
	k8sclient = clientset
	metricsclient = metricsclientset
	return nil
}

func loadGroups() {
	pods = &podGroup{}
	pods.groups = make([]*Pod, 0)
	deployments = &deploymentGroup{}
	deployments.groups = make([]*Deployment, 0)
	statefulsets = &statefulsetGroup{}
	statefulsets.groups = make([]*Statefulset, 0)
}

const (
	Running          string = "Running"
	Pending          string = "Pending"
	Succeeded        string = "Succeeded"
	Failed           string = "Failed"
	Unknown          string = "Unknown"
	CrashLoopBackOff string = "CrashLoopBackOff"
	ImagePullBackOff string = "ImagePullBackOff"
	ErrImagePull     string = "ErrImagePull"
	Completed        string = "Completed"
)

const (
	MB int64 = 1024 * 1024 * 1024
)

const (
	abnormallyPods string = "Pod is abnormally"
)

const (
	NORMAL int = iota
	SCALE_UP
	SCALE_OUT
)
