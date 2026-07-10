package collector

import (
	"bufio"
	"os"
	"strconv"
	"strings"
)

type sysCPU struct {
	total float64
	idle  float64
}

var lastCPU = make(map[string]sysCPU)

func readHostCPU() []float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return nil
	}
	defer f.Close()

	var cores []float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "cpu") && line != "cpu" { // cpu0, cpu1...
			parts := strings.Fields(line)
			if len(parts) < 5 {
				continue
			}
			name := parts[0]
			var total, idle float64
			for i, p := range parts[1:] {
				v, _ := strconv.ParseFloat(p, 64)
				total += v
				if i == 3 || i == 4 { // idle or iowait
					idle += v
				}
			}

			last := lastCPU[name]
			deltaTotal := total - last.total
			deltaIdle := idle - last.idle

			percent := 0.0
			if deltaTotal > 0 {
				percent = (1.0 - deltaIdle/deltaTotal) * 100.0
			}
			cores = append(cores, percent)
			lastCPU[name] = sysCPU{total: total, idle: idle}
		}
	}
	return cores
}

func readHostRAM() (totalMB, usedMB float64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()

	var memTotal, memAvailable float64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				memTotal, _ = strconv.ParseFloat(parts[1], 64)
			}
		} else if strings.HasPrefix(line, "MemAvailable:") {
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				memAvailable, _ = strconv.ParseFloat(parts[1], 64)
			}
		}
	}
	totalMB = memTotal / 1024.0
	usedMB = (memTotal - memAvailable) / 1024.0
	return
}

func readHostUptime() float64 {
	f, err := os.Open("/proc/uptime")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if scanner.Scan() {
		parts := strings.Fields(scanner.Text())
		if len(parts) >= 1 {
			u, _ := strconv.ParseFloat(parts[0], 64)
			return u
		}
	}
	return 0
}
