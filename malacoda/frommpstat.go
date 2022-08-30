package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type stats_cpu struct {
	cpu_user       uint64
	cpu_nice       uint64
	cpu_sys        uint64
	cpu_idle       uint64
	cpu_iowait     uint64
	cpu_steal      uint64
	cpu_hardirq    uint64
	cpu_softirq    uint64
	cpu_guest      uint64
	cpu_guest_nice uint64
}

type stats_sirq struct {
	net_tx uint64
	net_rx uint64
	sched  uint64
}

type stats_cgroup struct {
	name        string
	cpupressure float32
	iopressure  float32
	mempressure float32
	systime     float64
	children    []string
}

/* copied from mpstat
* Since ticks may vary slightly from CPU to CPU, we'll want
* to recalculate itv based on this CPU's tick count, rather
* than that reported by the "cpu" line. Otherwise we
* occasionally end up with slightly skewed figures, with
* the skew being greater as the time interval grows shorter.
 */
func get_per_cpu_interval(scc *stats_cpu, scp *stats_cpu) uint64 {
	var ishift uint64
	ishift = 0

	if (scc.cpu_user - scc.cpu_guest) < (scp.cpu_user - scp.cpu_guest) {
		/*
		 * Sometimes the nr of jiffies spent in guest mode given by the guest
		 * counter in /proc/stat is slightly higher than that included in
		 * the user counter. Update the interval value accordingly.
		 */
		ishift += (scp.cpu_user - scp.cpu_guest) -
			(scc.cpu_user - scc.cpu_guest)
	}
	if (scc.cpu_nice - scc.cpu_guest_nice) < (scp.cpu_nice - scp.cpu_guest_nice) {
		/*
		 * Idem for nr of jiffies spent in guest_nice mode.
		 */
		ishift += (scp.cpu_nice - scp.cpu_guest_nice) -
			(scc.cpu_nice - scc.cpu_guest_nice)
	}

	/*
	* Workaround for CPU coming back online: With recent kernels
	* some fields (user, nice, system) restart from their previous value,
	* whereas others (idle, iowait) restart from zero.
	* For the latter we need to set their previous value to zero to
	* avoid getting an interval value < 0.
	* (I don't know how the other fields like hardirq, steal... behave).
	* Don't assume the CPU has come back from offline state if previous
	* value was greater than ULLONG_MAX - 0x7ffff (the counter probably
	* overflew).
	 */
	if (scc.cpu_iowait < scp.cpu_iowait) && (scp.cpu_iowait < (0xffffffffffffffff - 0x7ffff)) {
		/*
		* The iowait value reported by the kernel can also decrement as
		* a result of inaccurate iowait tracking. Waiting on IO can be
		* first accounted as iowait but then instead as idle.
		* Therefore if the idle value during the same period did not
		* decrease then consider this is a problem with the iowait
		* reporting and correct the previous value according to the new
		* reading. Otherwise, treat this as CPU coming back online.
		 */
		if (scc.cpu_idle > scp.cpu_idle) || (scp.cpu_idle >= (0xffffffffffffffff - 0x7ffff)) {
			scp.cpu_iowait = scc.cpu_iowait
		} else {
			scp.cpu_iowait = 0
		}
	}
	if (scc.cpu_idle < scp.cpu_idle) && (scp.cpu_idle < (0xffffffffffffffff - 0x7ffff)) {
		scp.cpu_idle = 0
	}

	/*
	* Don't take cpu_guest and cpu_guest_nice into account
	* because cpu_user and cpu_nice already include them.
	 */
	return ((scc.cpu_user + scc.cpu_nice +
		scc.cpu_sys + scc.cpu_iowait +
		scc.cpu_idle + scc.cpu_steal +
		scc.cpu_hardirq + scc.cpu_softirq) -
		(scp.cpu_user + scp.cpu_nice +
			scp.cpu_sys + scp.cpu_iowait +
			scp.cpu_idle + scp.cpu_steal +
			scp.cpu_hardirq + scp.cpu_softirq) +
		ishift)
}

func ll_sp_value(value1 uint64, value2 uint64, itv uint64) float64 {
	if value2 < value1 {
		return 0
	} else {
		return float64((value2)-(value1)) / float64((itv)) * 100
	}
}

/*
 ***************************************************************************
 * Read machine uptime, independently of the number of processors.
 *
 * OUT:
 * @uptime	Uptime value in hundredths of a second.
 ***************************************************************************
 */
func read_uptime(uptime *uint64) {
	fp, err := os.Open("/proc/uptime")
	check(err)

	var up_sec, up_cent uint64

	scanner := bufio.NewScanner(fp)
	scanner.Scan()
	fmt.Sscanf(scanner.Text(), "%d.%d",
		&up_sec, &up_cent)
	*uptime = up_sec*100 + up_cent
	fp.Close()
}

func get_interval(prev_uptime uint64, curr_uptime uint64) uint64 {
	var itv uint64

	/* prev_time=0 when displaying stats since system startup */
	itv = curr_uptime - prev_uptime

	if itv == 0 { /* Paranoia checking */
		itv = 1
	}

	return itv
}

/*
* Read from the loadavg file to get last minute load of the system
 */
func sampleLoadAvg(state *stateData) {
	fp, err := os.Open("/proc/loadavg")
	check(err)

	scanner := bufio.NewScanner(fp)
	scanner.Scan()
	fmt.Sscanf(scanner.Text(), "%f %*f %*f %*d/%*d %*d", &state.loadAvg)
	fp.Close()
}

/*
* Read from the stat file the jiffies of the overall cpu
* storing a few information compared to offered
 */
func sampleDataStat(cpustate *stats_cpu) {
	fp, err := os.Open("/proc/stat")
	check(err)

	scanner := bufio.NewScanner(fp)
	scanner.Scan()
	fmt.Sscanf(scanner.Text()[5:], "%d %d %d %d %d %d %d %d %d %d",
		&cpustate.cpu_user,
		&cpustate.cpu_nice,
		&cpustate.cpu_sys,
		&cpustate.cpu_idle,
		&cpustate.cpu_iowait,
		&cpustate.cpu_steal,
		&cpustate.cpu_hardirq,
		&cpustate.cpu_softirq,
		&cpustate.cpu_guest,
		&cpustate.cpu_guest_nice)
	fp.Close()
}

func sampleSoftIrq(sirq *stats_sirq) {
	fp, err := os.Open("/proc/softirqs")
	check(err)

	scanner := bufio.NewScanner(fp)
	sumNET_TX := 0
	sumNET_RX := 0
	sumSCHED := 0
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), "NET_TX") {
			elems := strings.Split(scanner.Text(), " ")
			for i := range elems {
				if elems[i] != " " {
					value, _ := strconv.Atoi(elems[i])
					if value > 0 {
						sumNET_TX += value
					}
				}
			}
			scanner.Scan()
			elems = strings.Split(scanner.Text(), " ")
			for i := range elems {
				if elems[i] != " " {
					value, _ := strconv.Atoi(elems[i])
					if value > 0 {
						sumNET_RX += value
					}
				}
			}
			sirq.net_rx = uint64(sumNET_RX)
			sirq.net_tx = uint64(sumNET_TX)
		} else if strings.Contains(scanner.Text(), "SCHED") {
			elems := strings.Split(scanner.Text(), " ")
			for i := range elems {
				if elems[i] != " " {
					value, _ := strconv.Atoi(elems[i])
					if value > 0 {
						sumSCHED += value
					}
				}
			}
			sirq.sched = uint64(sumSCHED)
		}
	}
}

func sampleCgroup(folder string) *stats_cgroup {
	var stats stats_cgroup
	stats.name = folder
	var val [4]byte
	fp, err := os.Open(folder + "/cpu.pressure")
	check(err)
	fp.ReadAt(val[:], 11)
	temp, _ := strconv.ParseFloat(string(val[:]), 32)
	stats.cpupressure = float32(temp)
	fp.Close()

	fp, err = os.Open(folder + "/io.pressure")
	check(err)
	fp.ReadAt(val[:], 11)
	temp, _ = strconv.ParseFloat(string(val[:]), 32)
	stats.cpupressure = float32(temp)
	fp.Close()

	fp, err = os.Open(folder + "/mem.pressure")
	check(err)
	fp.ReadAt(val[:], 11)
	temp, _ = strconv.ParseFloat(string(val[:]), 32)
	stats.cpupressure = float32(temp)
	fp.Close()

	f, err := os.Open(folder)
	check(err)
	files, err := f.Readdirnames(-1)
	check(err)
	f.Close()

	for i := range files {
		if strings.Contains(files[i], "kubepod") || strings.Contains(files[i], "docker") {
			stats.children = append(stats.children, files[i])
		}
	}
	return &stats
}
