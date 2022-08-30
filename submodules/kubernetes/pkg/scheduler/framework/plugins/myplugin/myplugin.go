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

package myplugin

import (
	"context"
	"fmt"
	"time"
    "math/rand"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/apimachinery/pkg/types"
	//"k8s.io/client-go/tools/cache"
	//corev1helpers "k8s.io/component-helpers/scheduling/corev1"
	//"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/framework"
	//"sigs.k8s.io/scheduler-plugins/pkg/apis/config"
	//"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling"
	//"sigs.k8s.io/scheduler-plugins/pkg/apis/scheduling/v1alpha1"
	//pgclientset "sigs.k8s.io/scheduler-plugins/pkg/generated/clientset/versioned"
	//pgformers "sigs.k8s.io/scheduler-plugins/pkg/generated/informers/externalversions"
	//"sigs.k8s.io/scheduler-plugins/pkg/util"
)

// Coscheduling is a plugin that schedules pods in a group.
type Myplugin struct {}

var _ framework.FilterPlugin = &Myplugin{}

const (
	// Name is the name of the plugin used in Registry and configurations.
	Name = "Myplugin"
)

// New initializes and returns a new Coscheduling plugin.
func New(_ runtime.Object, handle framework.Handle) (framework.Plugin, error) {
    fmt.Print("CIAOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOO\n\n\n\n\nMIO PLUGIN INN CREATO\n\n\n\n\n\n\nn")
	return &Myplugin{}, nil
}

func (cs *Myplugin) EventsToRegister() []framework.ClusterEvent {
	return []framework.ClusterEvent{
		{Resource: framework.Pod, ActionType: framework.Add},
	}
}

// Name returns name of the plugin. It is used in logs, etc.
func (cs *Myplugin) Name() string {
	return Name
}

// Less is used to sort pods in the scheduling queue in the following order.
// 1. Compare the priorities of Pods.
// 2. Compare the initialization timestamps of PodGroups or Pods.
// 3. Compare the keys of PodGroups/Pods: <namespace>/<podname>.
//func (cs *Myplugin) Less(podInfo1, podInfo2 *framework.QueuedPodInfo) bool {
//}

// PreFilter performs the following validations.
// 1. Whether the PodGroup that the Pod belongs to is on the deny list.
// 2. Whether the total number of pods in a PodGroup is less than its `minMember`.


// PostFilter is used to rejecting a group of pods if a pod does not pass PreFilter or Filter.
func (pl *Myplugin) Filter(ctx context.Context, cycleState *framework.CycleState, pod *v1.Pod, nodeInfo *framework.NodeInfo) *framework.Status {
    fmt.Print("CIAOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOOO\n\n\n\n\nMIO PLUGIN INN ESECUZIONE\n\n\n\n\n\n\nn")
	node := nodeInfo.Node()
	if node == nil {
		return framework.NewStatus(framework.Error, "node not found")
	}
	// Ignore parsing errors for backwards compatibility.
	s1 := rand.NewSource(time.Now().UnixNano())
    r1 := rand.New(s1)
    match := r1.Intn(100) % 2

	if match>0 {
	    fmt.Print("NNODO DA ESCLUDERE\n\n")
		return framework.NewStatus(framework.UnschedulableAndUnresolvable, "Il mio plugin ha dato falso")
	}

    fmt.Print("NODO APPROVATO\n\n")
	return nil
}