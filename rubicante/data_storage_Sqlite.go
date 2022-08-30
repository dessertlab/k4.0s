//go:build sqlite
// +build sqlite

package main

import (
	"database/sql"
	"fmt"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

/*
* This file contains all the functions to store
* and retrieve state data.
* These could be replaced by a database
 */

func initStorage() {
}

func checkErr(err error) {
	if err != nil {
		panic(err)
	}
}

// retrieve containers data from storage
func read_containers_data() []Container {
	var containers []Container
	DATABASE, err := sql.Open("sqlite3", "./dbtest")
	checkErr(err)
	// query
	rows, err := DATABASE.Query("SELECT * FROM containers")
	checkErr(err)

	for rows.Next() {
		var c Container
		var uid int
		err = rows.Scan(&uid, &c.name, &c.band, &c.prio, &c.core)
		checkErr(err)
		res, err := DATABASE.Query(fmt.Sprintf("SELECT * FROM tasks WHERE tasks.container =%d", uid))
		checkErr(err)
		for res.Next() {
			var t Task
			var cuid int
			err = res.Scan(&uid, &t.Wcet, &t.Period, &t.Prio, &cuid)
			checkErr(err)
			c.tasks = append(c.tasks, t)
		}
		res.Close()
		containers = append(containers, c)
	}
	rows.Close() //good habit to close
	DATABASE.Close()
	return containers
}

// function that updates the storage on a delete of a pod in order
// to free the allotted bandwidth on a future schedulability test
func deletePod(name string) []int {
	usedBandwidths := make([]int, nCPUs)
	state.m.Lock()
	for i := 0; i < len(state.containers); i++ {
		if state.containers[i].name != name {
			usedBandwidths[state.containers[i].core] += state.containers[i].band
		} else {
			//state.containers = append(state.containers[:i], state.containers[i+1:]...)
			state.containers = remove(state.containers, i)
			DATABASE, err := sql.Open("sqlite3", "./dbtest")
			checkErr(err)
			stmt, err := DATABASE.Prepare("delete from containers where name=?")
			checkErr(err)
			res, err := stmt.Exec(name)
			checkErr(err)
			_, err = res.RowsAffected()
			checkErr(err)
			//fmt.Println(affect)
			DATABASE.Close()
		}
	}
	//println("after delete there are containers :", len(state.containers))
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
	DATABASE, err := sql.Open("sqlite3", "./dbtest")
	checkErr(err)
	stmt, err := DATABASE.Prepare("insert into containers(name,band,prio,core) values(?,?,?,?) on conflict(name) do update set band=excluded.band")
	checkErr(err)
	state.m.Lock()
	if state.changed {
		for i := range state.containers {
			checkErr(err)
			//fmt.Println("BANDA", state.containers[i].band)
			res, err := stmt.Exec(state.containers[i].name,
				state.containers[i].band, state.containers[i].prio, state.containers[i].core)
			checkErr(err)
			id, _ := res.LastInsertId()
			if id != 0 {
				stmt2, err := DATABASE.Prepare("insert into tasks(wcet,period,prio,container) values(?,?,?,?)")
				checkErr(err)
				for _, t := range state.containers[i].tasks {
					_, err := stmt2.Exec(t.Wcet, t.Period, t.Prio, id)
					checkErr(err)
				}
			}
		}
		state.changed = false
	}
	DATABASE.Close()
	state.m.Unlock()
}

func dumpRoutine() {
	for {
		dumpState()
		time.Sleep(time.Second * 10)
	}
}
