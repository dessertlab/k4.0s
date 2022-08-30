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

package rtschedulability

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"strconv"
	"strings"

	"net/http"

	v1 "k8s.io/api/core/v1"
	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	// "k8s.io/client-go/kubernetes"
	// "k8s.io/client-go/rest"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// Coscheduling is a plugin that schedules pods in a group.
type Rtschedulability struct {
	urltable map[string]string
	iptable  map[string]string
	status   map[string][]int
}

var _ framework.PreFilterPlugin = &Rtschedulability{}
var _ framework.FilterPlugin = &Rtschedulability{}
var _ framework.PreBindPlugin = &Rtschedulability{}
var _ framework.ScorePlugin = &Rtschedulability{}

// preFilterState computed at PreFilter and used at Filter.
type preFilterState struct {
	FatSchedRequest
}

// Clone the prefilter state.
func (s *preFilterState) Clone() framework.StateData {
	return s
}

type Task struct {
	Period int
	Wcet   int
	Prio   int
}

type FatTask struct {
	Period int
	Wcethi map[string]int
	Wcetlo int
	Prio   int
}
type SchedRequest struct {
	Tasks       []Task
	Name        string
	Criticality string
}

type FatSchedRequest struct {
	Tasks []FatTask
	//Prio  int
	Name string
}

type NodeState []int

type SchedResult struct {
	Schedulable bool
	Bandwidth   int
	Core        int
	Error       string
}

type SchedResponse struct {
	SchedRes *SchedResult
	Status   []int
}

type BindResponse struct {
	Result *SchedResult
	TGID   int
}

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "Rtschedulability"
	//preFilterKey is the key used to store info about the rt requirements
	preFilterKey = "FatRTInterface"
)

// New initializes and returns a new Coscheduling plugin.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
	// argomento threshold util?
	pl := Rtschedulability{
		urltable: make(map[string]string),
		iptable:  make(map[string]string),
		status:   make(map[string][]int),
	}

	return &pl, nil
}

func (cs *Rtschedulability) EventsToRegister() []framework.ClusterEvent {
	return []framework.ClusterEvent{
		{Resource: framework.Pod, ActionType: framework.Delete},
		{Resource: framework.Node, ActionType: framework.Add | framework.UpdateNodeAllocatable},
	}
}

// Name returns name of the plugin. It is used in logs, etc.
func (cs *Rtschedulability) Name() string {
	return Name
}

// PreFilter invoked at the prefilter extension point.
// The function converts the json into a data structure to avoid repeated computation
// and sets the structure as cycle state
func (pl *Rtschedulability) PreFilter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod) *framework.Status {
	var fschedRequest FatSchedRequest
	jschedRequest, exist := pod.Annotations["schedRequest"]
	if !exist {
		fmt.Println("NO rt equirements")
		return nil //no rt requirements
	}
	jschedRequest = jschedRequest[0:len(jschedRequest)-1] + ", \"Name\": \"" + pod.Name + "\"}"
	if err := json.Unmarshal([]byte(jschedRequest), &fschedRequest); err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	} else {
		result := &preFilterState{fschedRequest}
		cycleState.Write(preFilterKey, result)
	}
	return nil
}

// PreFilterExtensions returns prefilter extensions, pod add and remove.
func (pl *Rtschedulability) PreFilterExtensions() framework.PreFilterExtensions {
	return nil
}

// Filter selects suitable nodes for scheduling
// A request REST is sent to RT nodes that will respond with the schedulability test result
// The node state should not be modified, in this phase all the nodes are queried in parallel
func (pl *Rtschedulability) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
	c, err := cycleState.Read(preFilterKey)
	if err != nil {
		// preFilterState doesn't exist, no rt requirements
		return nil
	}
	fschedRequest := c.(*preFilterState)
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}

	//get node address
	url, exists := pl.urltable[node.Name]
	if !exists {
		address := node.Status.Addresses[0].Address
		pl.iptable[node.Name] = address
		url = "http://" + address + ":8888/shedulabilityTest"
		pl.urltable[node.Name] = url
	}

	var schedRequest SchedRequest
	critcality, exist := pod.Labels["Criticality"]
	if !exist {
		// rt requirements but no criticality!
		return framework.NewStatus(framework.Error, "RT requests but no criticality found!")
	}

	//Modify request to pass only needed wcet
	ptrSched := fschedRequest.FatSchedReqtoSchedReq(critcality, node.Name)
	if ptrSched == nil {
		fmt.Println("No WCET found for the node!")
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "No WCET found for the node!")
	}
	schedRequest = *ptrSched
	schedRequestj, err := json.Marshal(schedRequest)
	if err != nil {
		// invalid format of the rt request
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	fmt.Println(string(schedRequestj))
	// fmt.Print("\n\n\n")
	//get sched request

	var jsonStr = []byte(schedRequestj)
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

	//get response
	var res SchedResponse

	if err := json.Unmarshal([]byte(body), &res); err != nil {
		fmt.Println(err.Error())
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if res.SchedRes.Schedulable {
		pl.status[node.Name] = res.Status
		return nil
	} else {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Not schedulable")
	}
}

//Modify request to pass only needed wcet, that is the wcet for the specified node or the lo WCET
func (fsr *FatSchedRequest) FatSchedReqtoSchedReq(criticality string, nodename string) *SchedRequest {
	var retval SchedRequest
	//retval.Prio = fsr.Prio
	retval.Name = fsr.Name
	retval.Criticality = criticality
	retval.Tasks = make([]Task, len(fsr.Tasks))
	for i, t := range fsr.Tasks {
		retval.Tasks[i] = *t.FatTasktoTask(criticality, nodename)
		if retval.Tasks[i].Wcet == 0 {
			return nil
		}
	}
	return &retval
}
func (ft *FatTask) FatTasktoTask(criticality string, nodename string) *Task {
	var t Task
	t.Prio = ft.Prio
	t.Period = ft.Period
	if criticality == "HI" {
		t.Wcet = ft.Wcethi[nodename]
	} else if criticality == "LO" {
		t.Wcet = ft.Wcetlo
	}
	return &t
}

// PreBind check before binding if the node still allows the pod schedulability.
// Indeed node state could have been changed during scheudling cycle
// In this phase the node should change its state and if suitable it should allot RT resources
func (pl *Rtschedulability) PreBind(ctx context.Context, cs *framework.CycleState, pod *v1.Pod, nodeName string) *framework.Status {
	c, err := cs.Read(preFilterKey)
	if err != nil {
		// preFilterState doesn't exist, no rt requirements
		return nil
	}
	fschedRequest := c.(*preFilterState)
	var schedRequest SchedRequest
	address := pl.iptable[nodeName]
	url := "http://" + address + ":8888/podBind"
	critcality, _ := pod.Labels["Criticality"]

	//Modify request to pass only needed wcet
	ptrSched := fschedRequest.FatSchedReqtoSchedReq(critcality, nodeName)
	if ptrSched == nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "No WCET found for the node!")
	}
	schedRequest = *ptrSched
	schedRequestj, err := json.Marshal(schedRequest)
	if err != nil {
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	// fmt.Println(schedRequest)
	// fmt.Print("\n\n\n")
	var jsonStr = []byte(schedRequestj)
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

	//get response
	var res BindResponse

	if err := json.Unmarshal([]byte(body), &res); err != nil {
		fmt.Println(err.Error())
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, err.Error())
	}
	if res.Result.Schedulable {
		pod.Annotations["TGID"] = strconv.Itoa(res.TGID)
		pod.Annotations["RTbandwidth"] = strconv.Itoa(res.Result.Bandwidth)
		pod.ObjectMeta.SetAnnotations(pod.Annotations)
		// config, _ := rest.InClusterConfig()
		// clientset, _ := kubernetes.NewForConfig(config)
		// pod, _ := clientset.CoreV1().Pods("default").Update(context.TODO(), pod, metav1.UpdateOptions{})
		//if it is still schedulable add env variable to bind quota group
		for i := 0; i < len(pod.Spec.Containers); i++ {
			envs := append(pod.Spec.Containers[i].Env, v1.EnvVar{Name: "TGID", Value: strconv.Itoa(res.TGID)})
			pod.Spec.Containers[i].Env = envs
			//fmt.Println(pod.Spec.Containers[i].Env)
			return nil
		}
		return nil
	} else {
		//else return false
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Not schedulable")
	}
}

// Score invoked at the score extension point.
// The score is computed as the max free bandwidth between cores of the nodes
func (pl *Rtschedulability) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	// To score nodes, we search for the minimum occupied rt core
	// The score is the free bandwidth on thtat core
	score := computeScore(pl.status[nodeName])
	fmt.Println(score)
	return score, nil
}

func computeScore(status []int) int64 {
	freeCore := 100
	for _, core := range status {
		if core < freeCore {
			freeCore = core
		}
	}
	return int64(100 - freeCore)
}

// ScoreExtensions of the Score plugin.
func (pl *Rtschedulability) ScoreExtensions() framework.ScoreExtensions {
	return pl
}

func (alloc *Rtschedulability) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
	// Find highest and lowest scores.
	var highest int64 = -math.MaxInt64
	var lowest int64 = math.MaxInt64
	for _, nodeScore := range scores {
		if nodeScore.Score > highest {
			highest = nodeScore.Score
		}
		if nodeScore.Score < lowest {
			lowest = nodeScore.Score
		}
	}

	// Transform the highest to lowest score range to fit the framework's min to max node score range.
	oldRange := highest - lowest
	newRange := framework.MaxNodeScore - framework.MinNodeScore
	for i, nodeScore := range scores {
		if oldRange == 0 {
			scores[i].Score = framework.MinNodeScore
		} else {
			scores[i].Score = ((nodeScore.Score - lowest) * newRange / oldRange) + framework.MinNodeScore
		}
	}
	return nil
}
