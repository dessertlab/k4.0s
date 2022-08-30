package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/julienschmidt/httprouter"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/informers"
	coreinformers "k8s.io/client-go/informers/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

/*
* This file implements the calls of the REST API together with the functions
* needed to startup the kubernetes client and update node spec
 */

type Task struct {
	Period int
	Wcet   int
	Prio   int
}

type SchedRequest struct {
	Tasks []Task
	//Prio        int
	Name        string
	Criticality string
}

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

// Treshold to deem workload schedulable (% of used CPU)
const (
	THRESHOLDSCHED = 80
)

// Global vars used to store the clients and Kubernetes related info
var NODE *v1.Node
var CLIENTSET *kubernetes.Clientset
var MYNODENAME string
var SCHEDULABLE bool

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, `"O Rubicante, fa che tu li metti
li unghioni a dosso, sì che tu lo scuoi!",
gridavan tutti insieme i maladetti.!
-- XXII canto vv. 40-42`)
}

func SchedulabilityTest(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	body, _ := ioutil.ReadAll(r.Body)
	var schedRequest SchedRequest
	var response SchedResponse

	if err := json.Unmarshal([]byte(body), &schedRequest); err != nil {
		response = SchedResponse{
			SchedRes: &SchedResult{
				Schedulable: false,
				Error:       err.Error(),
			},
			Status: make([]int, nCPUs),
		}
	} else {
		sort.SliceStable(schedRequest.Tasks, func(i, j int) bool {
			return schedRequest.Tasks[i].Prio < schedRequest.Tasks[j].Prio
		})
		//printState()
		state.m.Lock()
		response.SchedRes, response.Status = schedTest(schedRequest, false)
		state.m.Unlock()
		//printState()
		//fmt.Println(response.SchedRes.Schedulable)
	}

	if responseRouter, err := json.Marshal(response); err != nil {
		log.Fatalln(err)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseRouter)
	}
}

func PodBind(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	body, _ := ioutil.ReadAll(r.Body)
	var schedRequest SchedRequest
	var response BindResponse
	var usedBandwidths []int

	if err := json.Unmarshal([]byte(body), &schedRequest); err != nil {
		response.Result = &SchedResult{
			Schedulable: false,
			Error:       err.Error(),
		}
	} else {
		sort.SliceStable(schedRequest.Tasks, func(i, j int) bool {
			return schedRequest.Tasks[i].Prio < schedRequest.Tasks[j].Prio
		})

		//printState()
		state.m.Lock()
		response.Result, usedBandwidths = schedTest(schedRequest, true)
		state.m.Unlock()
		//printState()
		//fmt.Println(response.Result.Schedulable)

		if response.Result.Schedulable {
			//allocate resources for incoming pod
			response.TGID = allocateResources(response.Result)
		}
	}

	if responseRouter, err := json.Marshal(response); err != nil {
		log.Fatalln(err)
	} else {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(responseRouter)
	}
	fmt.Println(usedBandwidths)
	//updateSchedulability(&usedBandwidths)
}

// Set the node Unschedulable if all the core have passed the threshold
func updateSchedulability(usedBandwidths *[]int) {
	schedulable := false
	if usedBandwidths != nil {
		for _, cap := range *usedBandwidths {
			if cap < THRESHOLDSCHED {
				schedulable = true
			}
		}
	}
	if !schedulable && SCHEDULABLE {
		NODE.Spec.Unschedulable = true
		SCHEDULABLE = false
		_, UpdateErr := CLIENTSET.CoreV1().Nodes().Update(context.TODO(), NODE, metav1.UpdateOptions{})
		if UpdateErr != nil {
			fmt.Print(UpdateErr.Error())
			//return errors.New("strigna di errore")
		}
	} else if schedulable && !SCHEDULABLE {
		NODE.Spec.Unschedulable = false
		SCHEDULABLE = true
		_, UpdateErr := CLIENTSET.CoreV1().Nodes().Update(context.TODO(), NODE, metav1.UpdateOptions{})
		if UpdateErr != nil {
			fmt.Print(UpdateErr.Error())
			//return errors.New("strigna di errore")
		}
	}
}

func configClient() {
	MYNODENAME := os.Getenv("THIS_NODE")
	fmt.Println("Starting on node", MYNODENAME)
	config, err := rest.InClusterConfig()
	SCHEDULABLE = true

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

	nodes := clientset.CoreV1().Nodes()
	node, err := nodes.Get(context.TODO(), MYNODENAME, metav1.GetOptions{})

	if err != nil {
		fmt.Print("Couldn't find node ", MYNODENAME, err.Error())
		os.Exit(3)
	}

	CLIENTSET = clientset
	NODE = node
	return
}

// Function called when a pod is deleted to free the allotted bandwidth
func deleteWatcher() {
	// nodefilter := informers.WithTweakListOptions(func(opts *metav1.ListOptions) {
	// 	opts.FieldSelector = fmt.Sprintf("spec.nodeName=%s", MYNODENAME)
	// })

	// informerFactory := informers.NewSharedInformerFactoryWithOptions(CLIENTSET, time.Second*30, nodefilter)
	informerFactory := informers.NewSharedInformerFactory(CLIENTSET, time.Second*30)
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
}

func (c *PodLoggingController) podAdd(obj interface{})         {}
func (c *PodLoggingController) podUpdate(old, new interface{}) {}
func (c *PodLoggingController) podDelete(obj interface{}) {
	pod := obj.(*v1.Pod)
	fmt.Println("POD DELETED", pod.Name)
	usedBandwidths := deletePod(pod.Name)
	updateSchedulability(&usedBandwidths)
}

type PodLoggingController struct {
	informerFactory informers.SharedInformerFactory
	podInformer     coreinformers.PodInformer
}

// Couple of functions to allign milliCPU seen by kubernetes with RT CPU used
// func increaseNodeResources(bw int) {
// 	qty := resource.NewMilliQuantity(int64(bw*1000), resource.DecimalSI)
// 	NODE.Status.Allocatable.Cpu().Add(*qty)
// 	_, UpdateErr := CLIENTSET.CoreV1().Nodes().Update(context.TODO(), NODE, metav1.UpdateOptions{})
// 	if UpdateErr != nil {
// 		fmt.Print(UpdateErr.Error())
// 		//return errors.New("strigna di errore")
// 	}

// }

// func decreaseNodeResources(bw int) {
// 	qty := resource.NewMilliQuantity(int64(bw*1000), resource.DecimalSI)
// 	NODE.Status.Allocatable.Cpu().Sub(*qty)
// 	_, UpdateErr := CLIENTSET.CoreV1().Nodes().Update(context.TODO(), NODE, metav1.UpdateOptions{})
// 	if UpdateErr != nil {
// 		fmt.Print(UpdateErr.Error())
// 		//return errors.New("strigna di errore")
// 	}

// }
