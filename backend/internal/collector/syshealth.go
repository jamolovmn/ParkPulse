package collector

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func (c *Collector) systemHealthLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			cores := readHostCPU()
			totalMB, usedMB := readHostRAM()
			uptime := readHostUptime()

			var conStats []ContainerStat

			opts := container.ListOptions{All: false}
			containers, err := c.cli.ContainerList(ctx, opts)
			if err == nil {
				for _, ctr := range containers {
					name := ctr.Names[0]
					if strings.HasPrefix(name, "/") {
						name = name[1:]
					}

					statsRes, err := c.cli.ContainerStats(ctx, ctr.ID, false)
					if err != nil {
						continue
					}
					var v types.StatsJSON
					if err := json.NewDecoder(statsRes.Body).Decode(&v); err != nil {
						statsRes.Body.Close()
						continue
					}
					statsRes.Body.Close()

					cpuDelta := float64(v.CPUStats.CPUUsage.TotalUsage) - float64(v.PreCPUStats.CPUUsage.TotalUsage)
					systemDelta := float64(v.CPUStats.SystemUsage) - float64(v.PreCPUStats.SystemUsage)
					cpuPercent := 0.0
					if systemDelta > 0.0 && cpuDelta > 0.0 {
						cpus := float64(v.CPUStats.OnlineCPUs)
						if cpus == 0.0 {
							cpus = float64(len(v.CPUStats.CPUUsage.PercpuUsage))
						}
						cpuPercent = (cpuDelta / systemDelta) * cpus * 100.0
					}

					ramPercent := 0.0
					if v.MemoryStats.Limit > 0 {
						ramPercent = (float64(v.MemoryStats.Usage) / float64(v.MemoryStats.Limit)) * 100.0
					}
					ramMB := float64(v.MemoryStats.Usage) / (1024 * 1024)

					conStats = append(conStats, ContainerStat{
						Name:   name,
						CPU:    cpuPercent,
						RAM:    ramPercent,
						RAM_MB: ramMB,
					})
				}
			}

			h := Health{
				UptimeSec:  uptime,
				Cores:      cores,
				Containers: conStats,
				TotalRAM:   totalMB,
				UsedRAM:    usedMB,
			}

			select {
			case c.HealthOut <- h:
			case <-ctx.Done():
				return
			default:
			}
		}
	}
}
