package main

import (
	"fmt"
	"os"
	"time"
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
	THRESHOLD_LOADAVG     = 6
	THRESHOLD_SYSTIME     = 15
	THRESHOLD_LOADAVG_DER = 2
	THRESHOLD_SYSTIME_DER = 10
)

func main() {
	var cstate, pstate stateData
	var scc, scp stats_cpu
	var cur_uptime, prev_uptime, itv uint64
	var cur_sirq, prev_sirq stats_sirq

	sampleDataStat(&scc)
	sampleLoadAvg(&cstate)
	sampleSoftIrq(&cur_sirq)

	f, _ := os.OpenFile("derivata.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0664)
	for i := 0; i < 1120; i++ {
		time.Sleep(2 * time.Second)
		prev_uptime = cur_uptime
		read_uptime(&cur_uptime)
		scp = scc
		prev_sirq = cur_sirq
		pstate = cstate
		sampleDataStat(&scc)
		sampleLoadAvg(&cstate)
		sampleSoftIrq(&cur_sirq)
		deltot_jiffies := get_per_cpu_interval(&scc, &scp)
		computestepCpu(&scc, &scp, deltot_jiffies, &cstate)
		itv = get_interval(prev_uptime, cur_uptime)
		computestepSirq(&cur_sirq, &prev_sirq, itv, &cstate)

		alarm = false
		valueThresholdCheck(&cstate)
		trendThresholdCheck(&cstate, &pstate, f)

		if alarm {
			secondStage()
		}
	}
	f.Close()
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
	return uint8(counter)
}

func trendThresholdCheck(state *stateData, prev *stateData, f *os.File) uint8 {
	deriv := computeDerivative(state, prev)
	counter := 0
	f.WriteString(fmt.Sprint(deriv.loadAvg, " ", deriv.systime, " ", deriv.iowait, " ", deriv.net, " ", deriv.sched, "\n"))
	if deriv.loadAvg > THRESHOLD_LOADAVG_DER {
		counter += 1
		alarm = true
	}
	if deriv.systime > THRESHOLD_SYSTIME_DER {
		counter += 1
		alarm = true
	}
	return uint8(counter)
}

func computeDerivative(state *stateData, prev *stateData) *stateData {
	var der stateData
	der.loadAvg = state.loadAvg - prev.loadAvg
	der.iowait = state.iowait - prev.iowait
	der.net = state.net - prev.net
	der.systime = state.systime - prev.systime
	der.sched = state.sched - prev.sched
	return &der
}

func secondStage() {
	fmt.Println("Alarm")
}
