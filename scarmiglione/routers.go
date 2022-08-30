package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/julienschmidt/httprouter"
	v1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
)

type Slice struct {
	Begin    int64 //us
	End      int64 //us
	PodName  string
	NodeName string
}

var NODE *v1.Node
var CLIENTSET *kubernetes.Clientset
var MYNODENAME string

func Index(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	fmt.Fprint(w, `Ei chinavan li raffi e "Vuo' che 'l tocchi",
diceva l'un con l'altro, "in sul groppone?".
E rispondien: "Sì, fa che gliel'accocchi".

Ma quel demonio che tenea sermone
col duca mio, si volse tutto presto
e disse: "Posa, posa, Scarmiglione!"
-- XXI canto vv. 100-105`)
}

func TDMAsched(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	body, _ := ioutil.ReadAll(r.Body)
	var schedRequest Slice
	response := "OK"
	if err := json.Unmarshal([]byte(body), &schedRequest); err != nil {
		response = "Bad parsing"
	}

	fmt.Println(schedRequest.Begin, schedRequest.End, schedRequest.PodName)
	//call tdmaconfig to add the slot
	//
	//

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

func TDMAremove(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	body, _ := ioutil.ReadAll(r.Body)
	var schedRequest Slice
	response := "OK"
	if err := json.Unmarshal([]byte(body), &schedRequest); err != nil {
		response = "Bad parsing"
	}

	//call tdmaconfig to remove the slot
	//
	//

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(response))
}

// func configClient() {
// 	MYNODENAME := os.Getenv("THIS_NODE")
// 	fmt.Print(MYNODENAME)
// 	config, err := rest.InClusterConfig()

// 	if err != nil {
// 		fmt.Print("Inclusterconfig not working\n")
// 		// No in cluster? letr's try locally
// 		kubehome := filepath.Join(homedir.HomeDir(), ".kube", "config")
// 		config, err = clientcmd.BuildConfigFromFlags("", kubehome)
// 		if err != nil {
// 			fmt.Printf("error loading kubernetes configuration: %s", err)
// 			os.Exit(1)
// 		}
// 	}

// 	clientset, err := kubernetes.NewForConfig(config)

// 	if err != nil {
// 		fmt.Printf("error creating kubernetes client: %s\n", err)
// 		os.Exit(1)
// 	}

// 	nodes := clientset.CoreV1().Nodes()
// 	fmt.Println(nodes)
// 	node, err := nodes.Get(context.TODO(), MYNODENAME, metav1.GetOptions{})

// 	if err != nil {
// 		fmt.Print("Couldn't find node ", MYNODENAME)
// 		os.Exit(3)
// 	}

// 	CLIENTSET = clientset
// 	NODE = node
// 	return
// }

// type PodLoggingController struct {
// 	informerFactory informers.SharedInformerFactory
// 	podInformer     coreinformers.PodInformer
// }

// func deleteWatcher() {
// 	informerFactory := informers.NewSharedInformerFactory(CLIENTSET, time.Second*30)

// 	podInformer := informerFactory.Core().V1().Pods()

// 	c := &PodLoggingController{
// 		informerFactory: informerFactory,
// 		podInformer:     podInformer,
// 	}
// 	podInformer.Informer().AddEventHandler(
// 		// Your custom resource event handlers.
// 		cache.ResourceEventHandlerFuncs{
// 			// Called on creation
// 			AddFunc: c.podAdd,
// 			// Called on resource update and every resyncPeriod on existing resources.
// 			UpdateFunc: c.podUpdate,
// 			// Called on resource deletion.
// 			DeleteFunc: c.podDelete,
// 		},
// 	)

// 	// add event handling for serviceInformer
// 	informerFactory.Start(wait.NeverStop)
// 	informerFactory.WaitForCacheSync(wait.NeverStop)
// }

// func (c *PodLoggingController) podAdd(obj interface{})         {}
// func (c *PodLoggingController) podUpdate(old, new interface{}) {}
// func (c *PodLoggingController) podDelete(obj interface{}) {
// 	pod := obj.(*v1.Pod)
// 	fmt.Println("POD DELETED", pod.Name)
// }
