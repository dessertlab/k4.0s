package main

import (
	"fmt"
	"log"
	"math/rand"
	"net/http"
	"time"

	"github.com/julienschmidt/httprouter"
	"k8s.io/klog"
)

func init() {
	rand.Seed(time.Now().UTC().UnixNano())
}

func main() {
	if err := start(); err != nil {
		klog.Fatalf("Failed to run Scarmiglione sscheduling daemon: %+v!", err)
		fmt.Println("Failed to run Scarmiglione sscheduling daemon!")
	}
}

func start() error {
	router := httprouter.New()
	router.GET("/", Index)
	router.POST("/TDMAsched", TDMAsched)
	router.POST("/TDMAremove", TDMAremove)

	log.Fatal(http.ListenAndServe(":8889", router))

	return nil
}
