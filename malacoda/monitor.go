package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"
)

type stateData struct {
	loadAvg float32
	systime float32
	iowait  float32
	net     float32
	sched   float32
}

var alarm bool

const (
	THRESHOLD_LOADAVG = 6
	THRESHOLD_SYSTIME = 15
	THRESHOLD_IOWAIT  = 15
	THRESHOLD_SCHED   = 15
	THRESHOLD_NET     = 15

	// 2 sec period, rescale if needed
	THRESHOLD_LOADAVG_DER = 0.24
	THRESHOLD_SYSTIME_DER = 6
	THRESHOLD_IOWAIT_DER  = 6
	THRESHOLD_NET_DER     = 1000
	THRESHOLD_SCHED_DER   = 170

	HISTORY_SIZE       = 5
	POSITIVE_THRESHOLD = 20
)

func phaseOne(period time.Duration) {
	var cstate, pstate, positiveFor stateData
	var scc, scp stats_cpu
	var cur_uptime, prev_uptime, itv uint64
	var cur_sirq, prev_sirq stats_sirq

	sampleDataStat(&scc)
	sampleLoadAvg(&cstate)
	sampleSoftIrq(&cur_sirq)
	for {
		time.Sleep(period * time.Second)
		prev_uptime = cur_uptime
		read_uptime(&cur_uptime)
		scp = scc
		pstate = cstate
		prev_sirq = cur_sirq
		sampleDataStat(&scc)
		sampleLoadAvg(&cstate)
		sampleSoftIrq(&cur_sirq)
		deltot_jiffies := get_per_cpu_interval(&scc, &scp)
		computestepCpu(&scc, &scp, deltot_jiffies, &cstate)
		itv = get_interval(prev_uptime, cur_uptime)
		computestepSirq(&cur_sirq, &prev_sirq, itv, &cstate)

		alarm = false

		counter := valueThresholdCheck(&cstate)
		counter += trendThresholdCheck(&cstate, &pstate, &positiveFor)

		if alarm {
			go secondStage(counter, 10)
		}
	}
}
func check(e error) {
	if e != nil {
		panic(e)
	}
}

/*
*	Computes the difference field by field between current and past state of the cpu
*   Returns a poitner to a new struct that contains the differences
 */
func diffCpuState(ccpu *stats_cpu, pcpu *stats_cpu) *stats_cpu {
	var diff stats_cpu
	diff.cpu_user = ccpu.cpu_user - pcpu.cpu_user
	diff.cpu_sys = ccpu.cpu_sys - pcpu.cpu_sys
	diff.cpu_nice = ccpu.cpu_nice - pcpu.cpu_nice
	diff.cpu_iowait = ccpu.cpu_iowait - pcpu.cpu_iowait
	diff.cpu_hardirq = ccpu.cpu_hardirq - pcpu.cpu_hardirq
	return &diff
}

/*
*	Computes the percentages as currentval - previous val / total ticks * 100
* 	of the fields contained in stats_cpu and fill stateData with the percentages
 */
func computestepCpu(scc *stats_cpu, scp *stats_cpu, delta uint64, state *stateData) {
	state.iowait = float32(ll_sp_value(scp.cpu_iowait, scc.cpu_iowait, delta))
	state.systime = float32(ll_sp_value(scp.cpu_sys, scc.cpu_sys, delta))
}

/*
*	Computes the interrupts per second as currentval - previous val / total seconds
* 	of the fields contained in stats_sirq and fill stateData with the percentages
 */
func computestepSirq(scc *stats_sirq, scp *stats_sirq, delta uint64, state *stateData) {
	tx := float32(ll_sp_value(scp.net_tx, scc.net_tx, delta))
	rx := float32(ll_sp_value(scp.net_rx, scc.net_rx, delta))
	state.net = tx + rx
	state.sched = float32(ll_sp_value(scp.sched, scc.sched, delta))
}

/*
*	The first checker is based on the absolute value of the parameters
*	if they are too high, alarm is triggered and the counter returned signals
*	how many parameters are above threshold
 */
func valueThresholdCheck(state *stateData) uint8 {
	counter := 0
	if state.loadAvg > THRESHOLD_LOADAVG {
		counter += 1
		alarm = true
	}
	if state.systime > THRESHOLD_SYSTIME {
		counter += 1
		alarm = true
	}
	if state.iowait > THRESHOLD_IOWAIT {
		counter += 1
		alarm = true
	}
	if state.net > THRESHOLD_NET {
		counter += 1
		alarm = true
	}
	if state.sched > THRESHOLD_SCHED {
		counter += 1
		alarm = true
	}
	return uint8(counter)
}

/*
*	Static variables needed to trendchecker
*	history is the history of the past steps derivative, head is where insert newvalue
 */
var history [HISTORY_SIZE]*stateData
var head = 0

/*
*	The second checker is based on the first derivative of the parameters
*	if the derivative is above threshold alarm is triggered and the counter returned signals
*	how many parameters are above threshold. The derivative value is taken as the average over the history
*	The checker also triggers alarms if the derivative is positive for too many steps looking into the history
 */
func trendThresholdCheck(state *stateData, prev *stateData, positiveFor *stateData) uint8 {
	/* Implementation 1: compute difference at each step and keep history of the differences
	* the average of these differences is the value used for thresholding */
	// deriv := computeDerivative(state, prev)
	// history[head] = deriv
	// head = (head + 1) % HISTORY_SIZE
	// average := computeAverage(history[:])
	/* Implementation 2: keep history of monitored value and compute directly
	* the difference between first and last value, divide and use the value for thresholding */
	history[head] = state
	head = (head + 1) % HISTORY_SIZE
	average := computeDifference(history[:], head)

	counter := 0
	//fmt.Println(deriv.loadAvg, " ", deriv.systime, " ", deriv.iowait, " ", deriv.net, " ", deriv.sched)

	if average.loadAvg > 0 {
		positiveFor.loadAvg += 1
		if positiveFor.loadAvg > POSITIVE_THRESHOLD || average.loadAvg > THRESHOLD_LOADAVG_DER {
			counter += 1
			alarm = true
		}
	} else {
		positiveFor.loadAvg = 0
	}

	if average.systime > 0 {
		positiveFor.systime += 1
		if positiveFor.systime > POSITIVE_THRESHOLD || average.systime > THRESHOLD_SYSTIME_DER {
			counter += 1
			alarm = true
		}
	} else {
		positiveFor.systime = 0
	}

	if average.iowait > 0 {
		positiveFor.iowait += 1
		if positiveFor.iowait > POSITIVE_THRESHOLD || average.iowait > THRESHOLD_IOWAIT_DER {
			counter += 1
			alarm = true
		}
	} else {
		positiveFor.iowait = 0
	}

	if average.net > 0 {
		positiveFor.net += 1
		if positiveFor.net > POSITIVE_THRESHOLD || average.net > THRESHOLD_NET_DER {
			counter += 1
			alarm = true
		}
	} else {
		positiveFor.net = 0
	}

	if average.sched > 0 {
		positiveFor.sched += 1
		if positiveFor.sched > POSITIVE_THRESHOLD || average.sched > THRESHOLD_SCHED_DER {
			counter += 1
			alarm = true
		}
	} else {
		positiveFor.sched = 0
	}

	return uint8(counter)
}

/*
*	Computes the first derivative of the signals as the difference between steps
 */
func computeDerivative(state *stateData, prev *stateData) *stateData {
	var der stateData
	der.loadAvg = state.loadAvg - prev.loadAvg
	der.iowait = state.iowait - prev.iowait
	der.net = state.net - prev.net
	der.systime = state.systime - prev.systime
	der.sched = state.sched - prev.sched
	return &der
}

/*
*	Computes difference between least and latest element of the history, divided by HISTORY_SIZE
 */
func computeAverage(history []*stateData) *stateData {
	var sum stateData
	for i := 0; i < HISTORY_SIZE; i++ {
		sum.loadAvg += history[i].loadAvg
		sum.iowait += history[i].iowait
		sum.net += history[i].net
		sum.systime += history[i].systime
		sum.sched += history[i].sched
	}
	sum.loadAvg /= HISTORY_SIZE
	sum.iowait /= HISTORY_SIZE
	sum.net /= HISTORY_SIZE
	sum.systime /= HISTORY_SIZE
	sum.sched /= HISTORY_SIZE
	return &sum
}

func computeDifference(history []*stateData, head int) *stateData {
	var sum stateData
	first := (head + 1) % HISTORY_SIZE
	sum.loadAvg = history[head].loadAvg - history[first].loadAvg
	sum.iowait = history[head].iowait - history[first].iowait
	sum.net = history[head].net - history[first].net
	sum.systime = history[head].systime - history[first].systime
	sum.sched = history[head].sched - history[first].sched

	sum.loadAvg /= HISTORY_SIZE
	sum.iowait /= HISTORY_SIZE
	sum.net /= HISTORY_SIZE
	sum.systime /= HISTORY_SIZE
	sum.sched /= HISTORY_SIZE
	return &sum
}

type suspected struct {
	pid uint32
	//path string
	pod  string
	rank uint16
}

type suspectedGroups struct {
	pid uint32
	//path string
	pod  string
	rank uint16
}

/*
*	The second stage of the monitor
*	Its aim is to find the guilty for the load and kill him
*	then it requests an eviction
 */
func secondStage(counter uint8, period time.Duration) {
	fmt.Println("Alarm")
	var susp []*stats_cgroup
	for {
		time.Sleep(period * time.Second)
		susp = list_suspected()
		for i := range susp {
			fmt.Println(susp[i].name)
		}
	}
}

func list_suspected() []*stats_cgroup {
	susp := list_suspected_helper("/sys/fs/cgroup/kubepods.slice")
	return susp
}
func list_suspected_helper(folder string) []*stats_cgroup {
	var stats *stats_cgroup
	var susp []*stats_cgroup
	stats = sampleCgroup(folder)
	if (*stats).isSuspect() {
		if len(stats.children) == 0 {
			susp = append(susp, stats)
		} else {
			for i := range stats.children {
				susp = append(susp, list_suspected_helper(stats.name+"/"+stats.children[i])...)
			}
		}
	}
	return susp
}

func (cg *stats_cgroup) isSuspect() bool {
	return true
}

var CLIENT *kubernetes.Clientset

func requestEviction(cg *stats_cgroup) {
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

	CLIENT, err = kubernetes.NewForConfig(config)
	pods, err := CLIENT.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{
		FieldSelector: "spec.nodeName=" + NODENAME})
	check(err)
	if err != nil {
		fmt.Printf("error creating kubernetes client: %s\n", err)
		os.Exit(1)
	}

}

func getPod(cgroup string) string {
	index := strings.Index(cgroup, "-pod")
	podid := cgroup[index + 4: index+4+36]

	for i := range pods()
}
