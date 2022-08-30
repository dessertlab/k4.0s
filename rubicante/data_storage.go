// +build !sqlite

package main

import (
	"encoding/csv"
	"fmt"
	"log"
	"os"
	"strconv"
	"time"
)

/*
* This file contains all the functions to store
* and retrieve state data.
* These could be replaced by a database
 */

func initStorage() {
}

// retrieve containers data from storage
func read_containers_data() []Container {
	f, err := os.OpenFile("containers.csv", os.O_CREATE|os.O_RDONLY, 0664)
	check(err)
	csvReader := csv.NewReader(f)
	data, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	f.Close()
	var containers []Container

	for _, line := range data {
		//if i > 0 { // omit header line
		var rec Container
		rec.period = SERVERPERIOD
		//fields are stored in this order:
		// name,bandwidth,priority,core
		// the period is fixed in this inmplementation
		rec.name = line[0]
		value, _ := strconv.Atoi(line[1])
		rec.band = value
		value, _ = strconv.Atoi(line[2])
		rec.prio = value
		value, _ = strconv.Atoi(line[3])
		rec.core = value
		rec.tasks = read_task_data(rec.name)
		// if(c.name == row[0]):
		// print("Error, a container with this name is already running!")
		// exit(1)
		containers = append(containers, rec)
		//}
	}
	return containers
}
func read_task_data(name string) []Task {
	f, err := os.OpenFile("tasks.csv", os.O_CREATE|os.O_RDONLY, 0664)
	check(err)
	csvReader := csv.NewReader(f)
	data, err := csvReader.ReadAll()
	if err != nil {
		log.Fatal(err)
	}
	f.Close()
	var tasks []Task

	for _, line := range data {
		//if i > 0 { // omit header line
		var rec Task
		//fields are stored in this order:
		// Containername,wcet,period,priority
		if line[0] == name {
			value, _ := strconv.Atoi(line[1])
			rec.Wcet = value
			value, _ = strconv.Atoi(line[2])
			rec.Period = value
			value, _ = strconv.Atoi(line[3])
			rec.Prio = value
		}
		tasks = append(tasks, rec)
		//}
	}
	return tasks
}

// function that updates the storage on a delete of a pod in order
// to free the allotted bandwidth on a future schedulability test
func deletePod(name string) []int {
	usedBandwidths := make([]int, nCPUs)
	state.m.Lock()
	for i := range state.containers {
		if state.containers[i].name != name {
			usedBandwidths[state.containers[i].core] += state.containers[i].band
		} else {
			//increaseNodeResources(state.containers[i].band)
			//state.containers = append(state.containers[:i], state.containers[i+1:]...)
			state.containers = remove(state.containers, i)
		}
	}
	state.m.Unlock()
	return usedBandwidths
}

func remove(s []Container, i int) []Container {
	s[i] = s[len(s)-1]
	return s[:len(s)-1]
}

func printState() {
	for c := range state.containers {
		fmt.Println(state.containers[c].name, " ", state.containers[c].band, " ",
			state.containers[c].prio, " ", state.containers[c].core, " ")
		fmt.Println("Tasklist:")
		for t := range state.containers[c].tasks {
			fmt.Println(state.containers[c].tasks[t].Period, " ", state.containers[c].tasks[t].Wcet)
		}
	}
}

func dumpState() {
	state.m.Lock()
	if state.changed {
		f, err := os.OpenFile("containers.csv", os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			log.Println(err)
		}
		defer f.Close()
		f2, err := os.OpenFile("tasks.csv", os.O_CREATE|os.O_WRONLY, 0664)
		if err != nil {
			log.Println(err)
		}
		defer f2.Close()
		for i := range state.containers {
			if _, err := f.WriteString(fmt.Sprintf("%s,%d,%d,%d\n", state.containers[i].name,
				state.containers[i].band, state.containers[i].prio, state.containers[i].core)); err != nil {
				log.Println(err)
			}
			for _, t := range state.containers[i].tasks {
				if _, err := f2.WriteString(fmt.Sprintf("%s,%d,%d,%d\n", state.containers[i].name,
					t.Wcet, t.Period, t.Prio)); err != nil {
					log.Println(err)
				}
			}
		}
		state.changed = false
	}
	state.m.Unlock()
}

func dumpRoutine() {
	for {
		dumpState()
		time.Sleep(time.Second * 10)
	}
}
