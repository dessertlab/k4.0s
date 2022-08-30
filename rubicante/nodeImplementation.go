package main

import (
	"fmt"
	"math"
	"sort"
)

/*
* This file contains the implementations of the functions
* related to the behaviour of the node: here is implemented
* the appropriate schedulability test and the core management
 */

const (
	SERVERPERIOD = 2500
)

type Container struct {
	tasks  []Task
	band   int
	name   string
	period int //kernel defined constant
	prio   int
	core   int
}

// function that computes the sched test: given a set of tasks
// checks if it is schedulable with a given priority and does not
// undermines guarantees for already scheduled containers
// Implements the sched test from Davis and Burns (2005)
func schedTest(args SchedRequest, persistent bool) (*SchedResult, []int) {
	//convertWcet(&args)
	prioComputed := setPrio(&args)
	fmt.Println("Prio for pod ", args.Name, " :", prioComputed)
	c := &Container{
		tasks:  args.Tasks,
		band:   100,
		period: SERVERPERIOD,
		prio:   prioComputed,
		core:   0,
		name:   args.Name,
	}
	var result SchedResult
	usedBandwidths := *getBandwidths()
	// find a core where the container is schedulable
	for core := 0; core < nCPUs; core = core + 1 {
		var cs []*Container
		for i := range state.containers {
			if state.containers[i].core == core {
				cs = append(cs, &state.containers[i])
			}
		}
		//fmt.Println("Formerly used bandwidth:", usedBandwidths[core], " core:", core)
		sched := c.binSearch(cs)
		// if it is schedulable, check that containers with lower prio are still schedulable
		if usedBandwidths[core]+c.band <= THRESHOLDSCHED && sched {
			cs = append(cs, c)
			sort.SliceStable(cs, func(i, j int) bool {
				return cs[i].prio > cs[j].prio
			})
			rollbackBandwidths := make([]int, len(cs))
			newbw := 0
			for i := range cs {
				rollbackBandwidths[i] = cs[i].band
				if cs[i].prio < c.prio {
					sched = cs[i].binSearch(cs)
					if !sched {
						break
					}
				}
				newbw += cs[i].band
			}
			// if schedulable, if we must save updates we append the container to the state
			// otherwise we rollback modification made to bandwidths
			if sched {
				c.core = core
				if persistent {
					state.containers = append(state.containers, *c)
					state.changed = true
				} else {
					for i := range cs {
						cs[i].band = rollbackBandwidths[i]
					}
				}
				usedBandwidths[core] = newbw
				fmt.Println("Currently used bandwidth: ", newbw)
				result = SchedResult{
					Schedulable: sched,
					Core:        core,
					Bandwidth:   c.band,
					Error:       "",
				}
				return &result, usedBandwidths
			}
			//rollback
			for i := range cs {
				cs[i].band = rollbackBandwidths[i]
			}
		}
	}
	result = SchedResult{
		Schedulable: false,
		Core:        0,
		Bandwidth:   0,
		Error:       "Not enough CPU bandwidth",
	}
	return &result, usedBandwidths
}

func convertWcet(args *SchedRequest) {
	if args.Criticality == "LO" {
		for _, t := range args.Tasks {
			t.Wcet = t.Wcet * WCETmultiplier
		}
	}
}

func setPrio(args *SchedRequest) int {
	min := args.Tasks[0].Period
	for i := range args.Tasks {
		if args.Tasks[i].Period < min {
			min = args.Tasks[i].Period
		}
	}
	sort.SliceStable(state.containers, func(i, j int) bool {
		return state.containers[i].getMinPeriod() < state.containers[j].getMinPeriod()
	})
	var i int
	for i = 0; i < len(state.containers); i++ {
		if min < state.containers[i].getMinPeriod() {
			break
		}
	}
	if i == 0 {
		return 128
	} else if i == len(state.containers) {
		return int(state.containers[i-1].prio / 2)
	} else if i > 0 {
		return int((state.containers[i].prio + state.containers[i-1].prio) / 2)
	} else {
		return int((255 - state.containers[0].prio) / 2)
	}
}
func (c *Container) getMinPeriod() int {
	min := c.tasks[0].Period
	for i := range c.tasks {
		if c.tasks[i].Period < min {
			min = c.tasks[i].Period
		}
	}
	return min
}

func getBandwidths() *[]int {
	usedBandwidths := make([]int, nCPUs)
	for i := range state.containers {
		usedBandwidths[state.containers[i].core] += state.containers[i].band
	}
	return &usedBandwidths
}

func check(e error) {
	if e != nil {
		panic(e)
	}
}

// This function search the minimum viable bandwidth for a container to be schedulable
// using the optimal bandwidth assignment from Davis and Burns (2008)
func (c *Container) binSearch(containers []*Container) bool {
	minb := 0
	allottedb := 0
	// useless to start at 100 as max
	for _, server := range containers {
		if server.prio > c.prio {
			allottedb += server.band
		}
	}
	maxb := 99 - allottedb
	for minb < maxb-1 {
		medb := math.Floor(float64(minb+maxb) / 2)
		c.band = int(medb)
		if c.checkSchedulability(containers) {
			maxb = int(medb)
		} else {
			minb = int(medb)
		}
	}
	maxb = maxb + 1 //overhead compensation
	c.band = maxb
	if c.checkSchedulability(containers) && maxb < THRESHOLDSCHED {
		return true
	} else {
		return false
	}
}

//check schedulability at task level
func (c *Container) checkSchedulability(containers []*Container) bool {
	pser := c.period
	cser := c.band * pser / 100
	if c.prio != -1 {
		for _, task := range c.tasks {
			if !task.checkSchedulability(containers, c.tasks, pser, cser, c.prio) {
				return false
			}
		}
		return true
	} else {
		return false
	}
}

// check task schedulability
func (t *Task) checkSchedulability(containers []*Container, tasks []Task, pser int, cser int, prioser int) bool {
	ctask := int(t.Wcet)
	ptask := int(t.Period)
	w0 := ctask + (int(math.Ceil(float64(ctask)/float64(cser))-1) * (pser - cser))
	wold := 0
	wnew := w0
	count := 0
	for wnew != wold {
		count += 1
		wold = wnew
		load_higher := 0
		for _, task := range tasks {
			if task.Prio > t.Prio {
				chigher := task.Wcet
				phigher := task.Period
				jhigher := task.Period - task.Wcet //in deferrable server case
				load_higher += (int(math.Ceil(float64((wold+jhigher))/float64(phigher))) * chigher)
			}
		}
		load := ctask + load_higher
		interference := 0
		for _, server := range containers {
			if server.prio > prioser {
				px := server.period
				cx := (server.band * px) / 100
				//jx := px - cx //in deferrable server case
				//interference += int(math.ceil( (max(0, wold - ((math.ceil(load/cser) - 1) * pser )) + jx)/px) * cx)
				interference += cx
			}
		}
		gaps := (int(math.Ceil(float64(load)/float64(cser))-1) * (pser - cser))
		wnew = int(load + gaps + interference)
		offset := (pser - cser)
		if wnew > (ptask - offset) {
			return false
		}
	}
	return true
}

// This function allocates resources for the rt-container at binding time
// In this case, we create a quota group and we return the TGID of the group
func allocateResources(sr *SchedResult) int {
	// create quota_group
	// cmd := exec.Command("./start")
	// err := cmd.Run()
	// fmt.Println("Finished:", err)
	// retrieve tgid and return it
	return 1
}

// func(self *Container) checkSchedulabilityServers(containers, prioser):  //check schedulability at server level
// pser = self.period
// cser = (self.band * pser)/100
// wold = 0
// wnew = cser
// while wnew != wold:                         #la analisi può essere ulteriormente semplificata a causa del fatto che i server sono bounded
// 	wold = wnew
// 	interference = 0
// 	for server in [s for s in containers if s.prio > prioser]:
// 		ci = (server.band * server.period)/100
// 		pi = server.period
// 		ji = pi - ci
// 		interference  += math.ceil((wold + ji)/pi) * ci
// 	wnew = cser + interference
// 	if wnew > pser:
// 		return False
// return True

// func assignBand(toSchedule SchedRequest, containers []Container) (bool, *[]Container) {
// 	cs := &[]Container{}
// 	bandsum := 0
// 	sort.SliceStable(toSchedule.tasks, func(i, j int) bool {
// 		return toSchedule.tasks[i].prio < toSchedule.tasks[j].prio
// 	})
// 	if !binSearch(c, cs) {
// 		return False, &[]Container{} //TODO error handling
// 	}

// 	bandsum += c.band
// 	cs = append(cs, c)
// 	if bandsum > 85 {
// 		return false, &[]Container{}
// 	} else {
// 		return true, cs
// 	}
// }

// sched := false
// min_group_prio := 0
// max_group_prio := 255
// if c.prio >= 0 {
// 	if c.checkSchedulabilityServers(containers, c.prio) {
// 		if c.checkSchedulability(containers, c.prio) {
// 			sched = true
// 		}
// 	}
// } else {
// 	for prioser := min_group_prio; prioser < max_group_prio; prioser++ {
// 		fmt.Println("Schedul test for prio ", prioser)
// 		if c.checkSchedulabilityServers(containers, prioser) {
// 			fmt.Println("Schedul test passed for servers for prio ", prioser)
// 			if c.checkSchedulability(containers, prioser) {
// 				fmt.Println("Schedul test passed for servers for tasks ", prioser)
// 				c.prio = prioser
// 				sched = true
// 				break
// 			}
// 		}
// 	}
// }

//schedulable, cs = assignBand(args, containers)
