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

package nodeassurancescorer

import (
	"context"
	"fmt"
	"math"
	"strconv"

	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/klog/v2"
	"k8s.io/kubernetes/pkg/scheduler/apis/config"
	"k8s.io/kubernetes/pkg/scheduler/framework"
)

// NodeAssuranceScorer is a score plugin that favors nodes based on their assurance
// level
type NodeAssuranceScorer struct {
	handle framework.Handle
	AssuranceScorer
}

type AssuranceScorer struct {
	Name string
	mode config.ModeType
	//annotation label?
}

var _ = framework.ScorePlugin(&NodeAssuranceScorer{})

// AllocatableName is the name of the plugin used in the Registry and configurations.
const AsssuranceScorerName = "NodeAssuranceScorer"

// Name returns name of the plugin. It is used in logs, etc.
func (alloc *NodeAssuranceScorer) Name() string {
	return AsssuranceScorerName
}

// Score invoked at the score extension point.
func (as *NodeAssuranceScorer) Score(ctx context.Context, state *framework.CycleState, pod *v1.Pod, nodeName string) (int64, *framework.Status) {
	nodeInfo, err := as.handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
	if err != nil {
		return 0, framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
	}

	// as.score favors nodes with least or most assurance level.
	// It calculates the sum of the node's weighted asssurance level
	//
	// Note: the returned "score" is negative for least, and positive for most .
	return as.score(pod, nodeInfo)
}

// ScoreExtensions of the Score plugin.
func (alloc *NodeAssuranceScorer) ScoreExtensions() framework.ScoreExtensions {
	return alloc
}

// NewAssuranceScorer initializes a new plugin and returns it.
func NewAssuranceScorer(assArgs runtime.Object, h framework.Handle) (framework.Plugin, error) {
	// Start with default values.
	mode := config.Least

	// Update values from args, if specified.
	if assArgs != nil {
		args, ok := assArgs.(*config.NodeAssuranceScorerArgs)
		if !ok {
			return nil, fmt.Errorf("want args to be of type NodeAssuranceScorerArgs, got %T", assArgs)
		}
		if args.Mode != "" {
			fmt.Println("MODE SPECIFIED", args.Mode)
			mode = args.Mode
			if mode != config.Least && mode != config.Most {
				return nil, fmt.Errorf("invalid mode, got %s", mode)
			}
		} else {
			fmt.Println("MODE UNNNSPECIFIED")
		}
	} else {
		fmt.Print("ARGS UNSPECIFIED")
	}

	return &NodeAssuranceScorer{
		handle: h,
		AssuranceScorer: AssuranceScorer{
			Name: AsssuranceScorerName,
			mode: mode,
		},
	}, nil
}

func getAssurance(node *v1.Node) (int, bool) {
	if len(node.Annotations) == 0 {
		klog.V(10).InfoS("Didn't found annotations", "nodeName", node.Name)
		return 0, false
	}

	annotations := node.Annotations
	values, exists := annotations["assurance"]
	value, err := strconv.Atoi(values)

	if err != nil {
		klog.V(10).InfoS("Found a non integer assurance", "nodeName", node.Name)
		return 0, false
	}
	if exists {
		if value >= 0 {
			return value, true
		} else {
			klog.V(10).InfoS("Found a negative assurance", "assuranceValue",
				value, "nodeName", node.Name)
			return 0, true
		}
	} else {
		klog.V(10).InfoS("Didn't found assurance", "nodeName", node.Name)
		return 0, false
	}
}

func (r *AssuranceScorer) score(
	pod *v1.Pod,
	nodeInfo *framework.NodeInfo) (int64, *framework.Status) {
	node := nodeInfo.Node()
	if node == nil {
		return 0, framework.NewStatus(framework.Error, "node not found")
	}

	tmpscore, _ := getAssurance(node)

	tempscore := int64(tmpscore)
	score := score(tempscore, r.mode)

	if klog.V(10).Enabled() {
		klog.InfoS("Node and score",
			"podName", pod.Name, "nodeName", node.Name,
			"score", tempscore)
	}
	fmt.Print("PLUGIN ASSURANCE SCORE", score)

	return score, nil
}

func score(assurance int64, mode config.ModeType) int64 {
	switch config.ModeType(mode) {
	case config.Least:
		return -1 * assurance
	case config.Most:
		return assurance
	}

	klog.V(10).InfoS("No match for mode", "mode", mode)
	return 0
}

// NormalizeScore invoked after scoring all nodes.
func (alloc *NodeAssuranceScorer) NormalizeScore(ctx context.Context, state *framework.CycleState, pod *v1.Pod, scores framework.NodeScoreList) *framework.Status {
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
