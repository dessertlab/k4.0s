package main

import (
	//"context"
	"fmt"
	"strconv"
	//"math"
	//k8s/io/kubernetes/pkg/api.Node
	"k8s.io/client-go/kubernetes"
	//v1 "k8s.io/api/core/v1"
	//"k8s.io/apimachinery/pkg/runtime"
	//"k8s.io/klog/v2"
	//"k8s.io/kubernetes/pkg/scheduler/framework"
	//"k8s.io/kubernetes/pkg/scheduler/apis/config"
)

func main() {
	if err := start(); err != nil {
		klog.Fatalf("Failed to run Malacoda criticality daemon: %+v", err)
	}
}


//clientset.CoreV1().Nodes().List(metav1.ListOptions{})
//pod, _ .= clientset.NodeV1beta1().Get(context.TODO(), "rtcase-hp-z230-tower-workstation", metav1.GetOptions{})
// for annotation_name, annotation_value := range pod.GetAnnotations(){
// 	fmt.Println(annotation_name, annotation_value)
// }

func start() error {

	var alpha = int
	var beta = int
	var gamma = int

	//retrieve info hw
	alpha = 0
	//retrieve info sw
	beta = 0
	for true {
		//ottenimento metriche

		//ottenimento assurance
		config, _ := rest.InClusterConfig()
		clientset, _ := kubernetes.NewForConfig(config)
		node, _ := clientset.CoreV1().Nodes().Get(context.TODO(), "rtcase-hp-z230-tower-workstation", metav1.GetOptions{})

		// handle framework.Handle
		// nodeInfo, err := handle.SnapshotSharedLister().NodeInfos().Get(nodeName)
		// if err != nil {
		// 	return framework.NewStatus(framework.Error, fmt.Sprintf("getting node %q from Snapshot: %v", nodeName, err))
		// }

		// node := nodeInfo.Node()
		// if node == nil {
		// 	return framework.NewStatus(framework.Error, "node not found")
		// }

		if len(node.Annotations) == 0 {
			klog.V(10).InfoS("Didn't found annotations", "nodeName", node.Name)
			return 0,false
		}

		annotations := node.Annotations
		values, exists := annotations["assurance"]
		gamma, err := strconv.Atoi(values)

		if klog.V(10).Enabled() {
			klog.InfoS("Node and assurance", "nodeName", node.Name, "assurance", gamma)
		}

		fmt.Print("DEBUG: valore assurance nodo", gamma)
		//funzione di calcolo

		assurance = alpha + beta + gamma
		var assuranceLev string 
		if assurance < 3 {
			assuranceLev = "NO"
		} else if assurance < 7 {
			assuranceLev = "LOW"
		} else {
			assuranceLev = "HI"
		}

		//aggiornamento

		if err != nil {
			return xerrors.Errorf("strigna di errore: %w", err)
		}

		return nil

	}
}