/*
Copyright 2020 The Kubernetes Authors.
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at
    http://www.apache.org/licenses/LICENSE-2.0
Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package tdmascheduling

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

type Slice struct {
	Begin    int64 //us
	End      int64 //us
	PodName  string
	NodeName string
}

type Req struct {
	Length int64 //us
	Cycle  int64 //us
}
type TDMAscheduling struct {
	slices     []Slice
	majorcycle int64 //us
	master     string
	port       int32
}

var _ framework.PreFilterPlugin = &TDMAscheduling{}
var _ framework.ReservePlugin = &TDMAscheduling{}

var globalPL *TDMAscheduling

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "TDMAscheduling"
)

// New initializes and returns a new TDMAscheduling plugin.
func New(plgArgs runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	go deleteWatcher()

	// start with default values
	var master string
	var port int32
	var cycle int64
	port = 8889
	master = "192.168.1.1"
	cycle = 5000
	// Update values from args, if specified.
	if plgArgs != nil {
		args, ok := plgArgs.(*config.TDMAschedulingArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type TDMAschedulingArgs, got %T", plgArgs)
		}
		if args.Master != "" {
			fmt.Println("TDMA master", args.Master)
			master = args.Master
		} else {
			fmt.Println("Master Unspecified")
		}
		if args.Cycle != 0 {
			fmt.Println("TDMA cycle", args.Cycle)
			cycle = args.Cycle
		} else {
			fmt.Println("Cycle Unspecified")
		}
		if args.Port != 0 {
			fmt.Println("TDMA port", args.Port)
			port = args.Port
		} else {
			fmt.Println("Cycle Unspecified")
		}
	} else {
		fmt.Print("ARGS UNSPECIFIED")
	}

	s := Slice{
		Begin: 0,
		End:   100,
	}
	pl := TDMAscheduling{
		slices:     []Slice{s},
		majorcycle: cycle, //us
		master:     master,
		port:       port,
	}

	globalPL = &pl

	return &pl, nil
}

func (cs *TDMAscheduling) EventsToRegister() []framework.ClusterEvent {
	return []framework.ClusterEvent{
		{Resource: framework.Pod, ActionType: framework.Delete},
		{Resource: framework.Node, ActionType: framework.Add | framework.UpdateNodeAllocatable},
	}
}

// Name returns name of the plugin. It is used in logs, etc.
func (pl *TDMAscheduling) Name() string {
	return Name
}

func (pl *TDMAscheduling) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	net, exist := pod.Labels["Network"]
	if !exist {
		// no network requirements
		return nil
	}
	fmt.Println("Looking for TDMA slice... prefilter")
	if net == "TDMA" {
		//updateTDMAstate(pl)
		var r Req
		reqj, exist := pod.Annotations["TDMA"]
		if !exist {
			fmt.Println("No TDMA annotation...")
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Network constraint, but requirements unspecified")
		}
		if err := json.Unmarshal([]byte(reqj), &r); err != nil {
			fmt.Println("Bad TDMA spec...")
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Bad TDMA specification, "+err.Error())
		}
		//atm descard cycle information ... future work
		for i := 0; i < len(pl.slices)-1; i++ {
			// schedulable
			if pl.slices[i+1].Begin-pl.slices[i].End > (r.Length + 20) {
				fmt.Println("Schedulable")
				return nil
			}
		}
		if pl.slices[len(pl.slices)-1].End < (pl.majorcycle - r.Length - 20) {
			fmt.Println("Schedulable at the end")
			return nil
		}
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "TDMA request cannot be satisfied")
	}
	fmt.Println("Unhandled network")
	//Unhandled case
	return nil
}

// check if pods that have a slot have been deleted from the last call, and update state
//func checkTDMAstate()

// PreFilterExtensions returns prefilter extensions, pod add and remove.
func (pl *TDMAscheduling) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

func (pl *TDMAscheduling) Reserve(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	net, exist := pod.Labels["Network"]
	if !exist {
		// no network requirements
		return nil
	}
	if net == "TDMA" {
		var r Req
		reqj, exist := pod.Annotations["TDMA"]
		if !exist {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Network constraint, but requirements unspecified")
		}
		if err := json.Unmarshal([]byte(reqj), &r); err != nil {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Bad TDMA specification, "+err.Error())
		}
		fmt.Println("Looking for TDMA slice...reserve")
		//atm descard cycle information ... future work
		scheduled := false
		for i := 0; i < len(pl.slices)-1; i++ {
			// schedulable
			if pl.slices[i+1].Begin-pl.slices[i].End > (r.Length + 20) {
				snew := Slice{
					Begin:    pl.slices[i].End + 10,
					End:      pl.slices[i].End + 10 + r.Length,
					PodName:  pod.Name,
					NodeName: nodeName,
				}
				err := reserveSlot(snew, pl)
				if err != nil {
					return err
				}
				pl.slices = append(pl.slices, snew)
				fmt.Println("Allotted slice:", snew.Begin, " ", snew.End)
				sort.SliceStable(pl.slices, func(h, k int) bool {
					return pl.slices[h].Begin < pl.slices[k].Begin
				})
				scheduled = true
			}
		}
		if !scheduled {
			if pl.slices[len(pl.slices)-1].End < (pl.majorcycle - r.Length - 20) {
				snew := Slice{
					Begin:    pl.slices[len(pl.slices)-1].End + 10,
					End:      pl.slices[len(pl.slices)-1].End + 10 + r.Length,
					PodName:  pod.Name,
					NodeName: nodeName,
				}
				err := reserveSlot(snew, pl)
				if err != nil {
					return err
				}
				pl.slices = append(pl.slices, snew)
				fmt.Println("Allotted slice:", snew.Begin, " ", snew.End)
				scheduled = true
			}
		}
		if !scheduled {
			return framework.NewStatus(framework.UnschedulableAndUnresolvable, "TDMA request cannot be satisfied")
		} else {
			return nil
		}
	}
	//Unhandled case
	return nil
}
func reserveSlot(s Slice, pl *TDMAscheduling) *framework.Status {
	//url of the tdma master node
	url := fmt.Sprintf("http://%s:%d/TDMAsched", pl.master, pl.port)
	sj, err := json.Marshal(s)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	var jsonStr = []byte(sj)
	req, err := http.NewRequest("POST", strings.TrimSpace(url), bytes.NewBuffer(jsonStr))
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	req.Header.Set("X-Custom-Header", "myvalue")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	if string(body) == "OK" {
		return nil
	} else {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, string(body))
	}
}

func (pl *TDMAscheduling) Unreserve(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) {
	net, exist := pod.Labels["Network"]
	if !exist {
		// no network requirements
		return
	}
	if net == "TDMA" {
		for i := 0; i < len(pl.slices)-1; i++ {
			if pl.slices[i].PodName == pod.Name {
				if i < len(pl.slices)-1 {
					pl.slices = append(pl.slices[:i], pl.slices[i+1:]...)
				} else {
					pl.slices = pl.slices[:len(pl.slices)-1]
				}
				err := unreserveSlot(pl.slices[i], pl)
				if err != nil {
					return
				}
			}
		}
	}
	//Unhandled case
	return
}

func unreserveSlot(s Slice, pl *TDMAscheduling) *framework.Status {
	//url of the tdma master node
	url := fmt.Sprintf("http://%s:%d/TDMAremove", pl.master, pl.port)
	sj, err := json.Marshal(s)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}

	var jsonStr = []byte(sj)
	req, err := http.NewRequest("POST", strings.TrimSpace(url), bytes.NewBuffer(jsonStr))
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	req.Header.Set("X-Custom-Header", "myvalue")
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Println(err.Error())
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	defer resp.Body.Close()

	body, _ := ioutil.ReadAll(resp.Body)
	fmt.Println("response Body:", string(body))
	if string(body) == "OK" {
		return nil
	} else {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, string(body))
	}
}

func deleteWatcher() {
	// MYNODENAME := os.Getenv("THIS_NODE")
	// fmt.Println("Starting on node", MYNODENAME)
	time.Sleep(6 * time.Second)
	config, err := rest.InClusterConfig()

	if err != nil {
		fmt.Println("Inclusterconfig not working!", err.Error())
		// No in cluster? letr's try locally
		kubehome := filepath.Join(homedir.HomeDir(), ".kube", "config")
		config, err = clientcmd.BuildConfigFromFlags("", kubehome)
		if err != nil {
			fmt.Printf("error loading kubernetes configuration: %s", err.Error())
			os.Exit(1)
		}
	}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		fmt.Printf("error creating kubernetes client: %s\n", err.Error())
		os.Exit(1)
	}

	informerFactory := informers.NewSharedInformerFactory(clientset, time.Second*30)
	podInformer := informerFactory.Core().V1().Pods()

	c := &PodLoggingController{
		informerFactory: informerFactory,
		podInformer:     podInformer,
	}
	podInformer.Informer().AddEventHandler(
		// Your custom resource event handlers.
		cache.ResourceEventHandlerFuncs{
			// Called on creation
			AddFunc: c.podAdd,
			// Called on resource update and every resyncPeriod on existing resources.
			UpdateFunc: c.podUpdate,
			// Called on resource deletion.
			DeleteFunc: c.podDelete,
		},
	)

	// add event handling for serviceInformer
	informerFactory.Start(wait.NeverStop)
	informerFactory.WaitForCacheSync(wait.NeverStop)
	return
}

func (c *PodLoggingController) podAdd(obj interface{})         {}
func (c *PodLoggingController) podUpdate(old, new interface{}) {}
func (c *PodLoggingController) podDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	if pod.Labels["Network"] == "TDMA" {
		fmt.Println("Deleted pod with TDMA")
		for i := 0; i < len(globalPL.slices); i++ {
			if globalPL.slices[i].PodName == pod.Name {
				unreserveSlot(globalPL.slices[i], globalPL)
				if i == len(globalPL.slices)-1 {
					globalPL.slices = globalPL.slices[:i]
				} else {
					globalPL.slices = append(globalPL.slices[:i], globalPL.slices[i+1:]...)
				}
			}
		}
	}
}

type PodLoggingController struct {
	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
}
