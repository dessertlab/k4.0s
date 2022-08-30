package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"

	"github.com/julienschmidt/httprouter"
	"k8s.io/klog"
)

type State struct {
	containers []Container
	m          sync.Mutex
	changed    bool
}

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	if err := start(); err != nil {
		klog.Fatalf("Failed to run Rubicante sscheduling daemon: %+v!", err)
		fmt.Println("Failed to run Rubicante sscheduling daemon!")
	}
}

var WCETmultiplier int
var nCPUs int
var state State

func start() error {

	initStorage()
	state.containers = read_containers_data()
	printState()
	go dumpRoutine()

	WCETmultiplier, err := strconv.Atoi(os.Getenv("WCETmultiplier"))
	if err != nil {
		WCETmultiplier = 1
	}
	fmt.Println("Using WCET multiplier:", WCETmultiplier)

	nCPUs = runtime.NumCPU()

	configClient()

	go deleteWatcher()

	router := httprouter.New()
	router.GET("/", Index)
	router.POST("/shedulabilityTest", SchedulabilityTest)
	router.POST("/podBind", PodBind)

	log.Fatal(http.ListenAndServe(":8888", router))

	return nil
}
