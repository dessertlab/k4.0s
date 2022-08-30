package main

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/sanposhiho/scheduler-playground/scheduler"
	"golang.org/x/xerrors"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	clientset "k8s.io/client-go/kubernetes"
	"k8s.io/klog/v2"

	"github.com/sanposhiho/scheduler-playground/config"
	"github.com/sanposhiho/scheduler-playground/k8sapiserver"
	"github.com/sanposhiho/scheduler-playground/pvcontroller"
	//"github.com/sanposhiho/scheduler-playground/scheduler/defaultconfig"
)

// entry point.
func main() {
	if err := start(); err != nil {
		klog.Fatalf("failed with error on running scheduler: %+v", err)
	}
}

// start starts scheduler and needed k8s components.
func start() error {
	cfg, err := config.NewConfig()
	if err != nil {
		return xerrors.Errorf("get config: %w", err)
	}

	restclientCfg, apiShutdown, err := k8sapiserver.StartAPIServer(cfg.EtcdURL)
	if err != nil {
		return xerrors.Errorf("start API server: %w", err)
	}
	defer apiShutdown()

	client := clientset.NewForConfigOrDie(restclientCfg)

	pvshutdown, err := pvcontroller.StartPersistentVolumeController(client)
	if err != nil {
		return xerrors.Errorf("start pv controller: %w", err)
	}
	defer pvshutdown()

	sched := scheduler.NewSchedulerService(client, restclientCfg)

	//sc, err := defaultconfig.DefaultSchedulerConfig()
	sc, err := config.GetSchedulerCfg()
	if err != nil {
		//return xerrors.Errorf("create scheduler config")
		return err
	}

	if err := sched.StartScheduler(sc); err != nil {
		return xerrors.Errorf("start scheduler: %w", err)
	}
	defer sched.ShutdownScheduler()

	err = scenario(client)
	if err != nil {
		return xerrors.Errorf("start scenario: %w", err)
	}

	return nil
}

func scenario(client clientset.Interface) error {
	ctx := context.Background()
    labels := map[string]string{"Criticality": "HI", "Network":"TDMA"}
	annotations := map[string]string{
		v1.PreferAvoidPodsAnnotationKey: `
							{
							    "preferAvoidPods": [
							        {
							            "podSignature": {
							                "podController": {
							                    "apiVersion": "v1",
							                    "kind": "ReplicaSet",
							                    "name": "foo",
							                    "uid": "qwert12345",
							                    "controller": true
							                }
							            },
							            "reason": "some reason",
							            "message": "some message"
							        }
							    ]
							}`,
	}

//"assurance": strconv.Itoa(17),
	// create node0 ~ node9, but all nodes are unschedulable
	for i := 0; i < 9; i++ {
	    annotations["assurance"] = strconv.Itoa(17*i)
		suffix := strconv.Itoa(i)
		_, err := client.CoreV1().Nodes().Create(ctx, &v1.Node{
			ObjectMeta: metav1.ObjectMeta{
				Name: "node" + suffix,
				Annotations: annotations,
				Labels: labels,
			},
			Spec: v1.NodeSpec{
				Unschedulable: true,
			},
			Status: v1.NodeStatus{
                Addresses: []v1.NodeAddress {
                    {
                        //Address : fmt.Sprintf("192.168.1.%d",i),
                        Address: "192.168.1.30",
                    },
                },
			},
		}, metav1.CreateOptions{})
		if err != nil {
			return fmt.Errorf("create node: %w", err)
		}
	}

	cpu, err := resource.ParseQuantity("4")
	mem, err := resource.ParseQuantity("32Gi")
	podnum, err := resource.ParseQuantity("10")

	// non unschedulable node
	_, err = client.CoreV1().Nodes().Create(ctx, &v1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: "node10",
			Annotations: annotations,
			Labels: labels,
		},
		Status: v1.NodeStatus{Capacity: v1.ResourceList{
			v1.ResourceCPU:    cpu,
			v1.ResourceMemory: mem,
			v1.ResourcePods:   podnum,
		},
		Addresses: []v1.NodeAddress {
                    {
                        Address : "192.168.1.30",
                    },
                },
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create node: %w", err)
	}

	klog.Info("scenario: all nodes created")
	time.Sleep(5 * time.Second)

	_, err = client.CoreV1().Pods("default").Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod1", Annotations: map[string]string{"schedRequest":`{
		"Tasks": [
			{
				"period": 100000,
				"wcethi":{"node10":3000},
				"wcetlo": 3000,
				"prio": 8
			},
			{
				"period": 12000,
				"wcethi":{"node10":10},
				"wcetlo": 10,
				"prio": 10
			},
			{
				"period": 2500,
				"wcethi":{"node10":50},
				"wcetlo": 50,
				"prio": 13
			}
		],
		"Prio": 6
	}`,
	"TDMA":`{"Length":100,"Cycle":4000}`,
	},Labels:labels, },
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "container1",
					Image: "k8s.gcr.io/pause:3.5",
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create pod: %w", err)
	}

	klog.Info("scenario: pod1 created")


	_, err = client.CoreV1().Pods("default").Create(ctx, &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "pod2", Annotations: map[string]string{"schedRequest":`{
		"Tasks": [
			{
				"period": 100000,
				"wcethi":{"node10":3000},
				"wcetlo": 3000,
				"prio": 8
			},
			{
				"period": 12000,
				"wcethi":{"node10":10},
				"wcetlo": 10,
				"prio": 10
			},
			{
				"period": 2500,
				"wcethi":{"node10":50},
				"wcetlo": 50,
				"prio": 13
			}
		],
		"Prio": 6
	}`,
	"TDMA":`{"Length":100,"Cycle":4000}`,
	},Labels:labels, },
		Spec: v1.PodSpec{
			Containers: []v1.Container{
				{
					Name:  "container1",
					Image: "k8s.gcr.io/pause:3.5",
				},
			},
		},
	}, metav1.CreateOptions{})
	if err != nil {
		return fmt.Errorf("create pod: %w", err)
	}

	klog.Info("scenario: pod2 created")

	time.Sleep(5 * time.Second)

	pod1, err := client.CoreV1().Pods("default").Get(ctx, "pod1", metav1.GetOptions{})
	if err != nil {
		return fmt.Errorf("get pod: %w", err)
	}
	klog.Info("pod1 is bound to " + pod1.Spec.NodeName)
    klog.Info("ANNOTATIONS: "+ pod1.Annotations["TGID"])
 klog.Info("ANNOTATIONS: "+ pod1.Annotations["RTbandwidth"])
	return nil
}
