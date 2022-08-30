package main

import (
	"context"
	"fmt"

	//"errors"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
	"k8s.io/klog/v2"
)

func main() {
	fmt.Println(`Malacoda daemon started!
"Nessun di voi sia fello!

Innanzi che l'uncin vostro mi pigli,
traggasi avante l'un di voi che m'oda,
e poi d'arruncigliarmi si consigli".

Tutti gridaron: "Vada Malacoda!"
--  XXI canto, vv. 72-75`)
	if err := start(); err != nil {
		klog.Fatalf("Failed to run Malacoda criticality daemon: %+v", err)
	}
}

var NODENAME string

func start() error {

	var alpha int
	var beta int

	annotString := "ASSURANCE"
	labelString := "Criticality"

	var period time.Duration
	period = 2 //sec

	//retrieve info hw
	alpha = 0
	//retrieve info sw
	beta = 0

	mynodename := os.Getenv("THIS_NODE")

	fmt.Print(mynodename)
	NODENAME = mynodename
	config, err := rest.InClusterConfig()

	//if err != nil {
	fmt.Print("Inclusterconfig not working\n")
	// No in cluster? letr's try locally
	kubehome := filepath.Join(homedir.HomeDir(), ".kube", "config")
	config, err = clientcmd.BuildConfigFromFlags("", kubehome)
	if err != nil {
		fmt.Printf("error loading kubernetes configuration: %s", err)
		os.Exit(1)
	}
	//}

	clientset, err := kubernetes.NewForConfig(config)

	if err != nil {
		fmt.Printf("error creating kubernetes client: %s\n", err)
		os.Exit(1)
	}

	nodes := clientset.CoreV1().Nodes()
	//list, _ := nodes.List(context.TODO(), metav1.ListOptions{})
	//fmt.Print(list.String())

	node, err := nodes.Get(context.TODO(), mynodename, metav1.GetOptions{})

	if err != nil {
		fmt.Print("Couldn't find node ", mynodename)
		os.Exit(3)
	}

	s1 := rand.NewSource(time.Now().UnixNano())
	for true {
		//gamma score computation
		r1 := rand.New(s1)
		gamma_new := r1.Intn(10)

		//retrieving assurance

		if len(node.Annotations) == 0 {
			klog.V(10).InfoS("Didn't found annotations", "nodeName", node.Name)
			fmt.Print("Didn't found annotations\n")
			//return errors.New("Didn't found annotations")
		}

		annotations := node.Annotations

		//gamma è frutto delle metriche rilevate
		values, exists := annotations[annotString]
		gamma, _ := strconv.Atoi(values)

		if exists {
			if gamma < 0 {
				klog.V(10).InfoS("Found a negative assurance", "assuranceValue",
					gamma, "nodeName", node.Name)
				fmt.Print("Found a negative assurance\n")
			}
		} else {
			klog.V(10).InfoS("Didn't found assurance", "nodeName", node.Name)
			fmt.Print("Didn't found assurance\n")
		}

		if klog.V(10).Enabled() {
			klog.InfoS("Node and assurance", "nodeName", node.Name, "assurance", gamma)
		}

		if gamma != gamma_new {
			gamma = gamma_new

			//assurance computation, criticality level labeling
			assurance := alpha + beta + gamma
			//fmt.Printf("DEBUG: valore assurance nodo %v \n", assurance)

			//updating values
			annotations[annotString] = strconv.Itoa(assurance)
			node.ObjectMeta.SetAnnotations(annotations)

			labels := node.Labels
			critvalue, exists := labels[labelString]

			if !exists {
				klog.V(10).InfoS("Didn't found criticality", "nodeName", node.Name)
				fmt.Print("Didn't found criticality\n")
				//return errors.New("Didn't found criticality")
			}

			var assuranceLev string
			if assurance < 3 {
				assuranceLev = "NO"
			} else if assurance < 7 {
				assuranceLev = "LOW"
			} else {
				assuranceLev = "HI"
			}

			if exists {
				if critvalue != assuranceLev {
					fmt.Print("CAMBIATO\n")
					//ordina un reschedule dei pod
					labels[labelString] = assuranceLev
					node.ObjectMeta.SetLabels(labels)
				}
			} else {
				labels[labelString] = assuranceLev
				node.ObjectMeta.SetLabels(labels)
			}

			_, UpdateErr := nodes.Update(context.TODO(), node, metav1.UpdateOptions{})

			if UpdateErr != nil {
				fmt.Print(err.Error())
				//return errors.New("strigna di errore")
			}
		}
		time.Sleep(period * time.Second)
	}
	return nil
}
