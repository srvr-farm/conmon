package collector

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

type cpuSnapshot struct {
	total uint64
	idle  uint64
}

func (c *HostCollector) Snapshot(ctx context.Context) (HostSample, error) {
	if err := ctx.Err(); err != nil {
		return HostSample{}, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	statData, err := fs.ReadFile(c.fs, "proc/stat")
	if err != nil {
		return HostSample{}, err
	}
	cpuStats, err := parseCPUStat(statData)
	if err != nil {
		return HostSample{}, err
	}

	memData, err := fs.ReadFile(c.fs, "proc/meminfo")
	if err != nil {
		return HostSample{}, err
	}
	totalMem, availableMem, err := parseMemInfo(memData)
	if err != nil {
		return HostSample{}, err
	}

	uptimeData, err := fs.ReadFile(c.fs, "proc/uptime")
	if err != nil {
		return HostSample{}, err
	}
	uptime, err := parseUptime(uptimeData)
	if err != nil {
		return HostSample{}, err
	}

	bootIDData, err := fs.ReadFile(c.fs, "proc/sys/kernel/random/boot_id")
	if err != nil {
		return HostSample{}, err
	}

	totalRatio, perCore := c.computeCPUUsage(cpuStats)

	resident := totalMem
	if resident > availableMem {
		resident -= availableMem
	} else {
		resident = 0
	}

	return HostSample{
		UptimeSeconds:       uptime,
		BootID:              strings.TrimSpace(string(bootIDData)),
		MemoryResidentBytes: resident,
		TotalCPUUsageRatio:  totalRatio,
		PerCoreUsageRatio:   perCore,
	}, nil
}

func parseCPUStat(data []byte) (map[string]cpuSnapshot, error) {
	stats := make(map[string]cpuSnapshot)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) < 5 || !strings.HasPrefix(fields[0], "cpu") {
			continue
		}
		values := make([]uint64, len(fields)-1)
		for i := range values {
			v, err := strconv.ParseUint(fields[i+1], 10, 64)
			if err != nil {
				return nil, fmt.Errorf("parse cpu stat %q: %w", fields[i+1], err)
			}
			values[i] = v
		}
		total := uint64(0)
		for _, v := range values {
			total += v
		}
		idle := uint64(0)
		if len(values) > 3 {
			idle += values[3]
		}
		if len(values) > 4 {
			idle += values[4]
		}
		stats[fields[0]] = cpuSnapshot{
			total: total,
			idle:  idle,
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return stats, nil
}

func parseMemInfo(data []byte) (uint64, uint64, error) {
	var totalKB, availKB *uint64
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, "MemTotal:"):
			if v, ok := parseMemLine(line); ok {
				totalKB = &v
			}
		case strings.HasPrefix(line, "MemAvailable:"):
			if v, ok := parseMemLine(line); ok {
				availKB = &v
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return 0, 0, err
	}
	if totalKB == nil || availKB == nil {
		return 0, 0, fmt.Errorf("meminfo missing fields")
	}
	return (*totalKB) * 1024, (*availKB) * 1024, nil
}

func parseMemLine(line string) (uint64, bool) {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0, false
	}
	v, err := strconv.ParseUint(fields[1], 10, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func parseUptime(data []byte) (float64, error) {
	fields := strings.Fields(string(data))
	if len(fields) == 0 {
		return 0, fmt.Errorf("uptime data empty")
	}
	val, err := strconv.ParseFloat(fields[0], 64)
	if err != nil {
		return 0, fmt.Errorf("parse uptime %q: %w", fields[0], err)
	}
	return val, nil
}

func (c *HostCollector) computeCPUUsage(curr map[string]cpuSnapshot) (float64, map[string]float64) {
	cloned := cloneCPUSnapshots(curr)
	if c.prev == nil {
		c.prev = cloned
		return 0, nil
	}

	totalCurr, okCurr := curr["cpu"]
	totalPrev, okPrev := c.prev["cpu"]
	totalRatio := 0.0
	if okCurr && okPrev {
		totalRatio = computeCPUUsageRatio(totalPrev, totalCurr)
	}

	var perCore map[string]float64
	if c.collectPerCore {
		perCore = make(map[string]float64, len(curr))
		for name, current := range curr {
			if name == "cpu" {
				continue
			}
			prev, ok := c.prev[name]
			if !ok {
				continue
			}
			perCore[name] = computeCPUUsageRatio(prev, current)
		}
		if len(perCore) == 0 {
			perCore = nil
		}
	}

	c.prev = cloned
	return totalRatio, perCore
}

func cloneCPUSnapshots(src map[string]cpuSnapshot) map[string]cpuSnapshot {
	dst := make(map[string]cpuSnapshot, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func computeCPUUsageRatio(prev, curr cpuSnapshot) float64 {
	if curr.total <= prev.total {
		return 0
	}
	deltaTotal := curr.total - prev.total
	var deltaIdle uint64
	if curr.idle >= prev.idle {
		deltaIdle = curr.idle - prev.idle
	} else {
		deltaIdle = curr.idle
	}
	if deltaIdle > deltaTotal {
		deltaIdle = deltaTotal
	}
	ratio := 1 - float64(deltaIdle)/float64(deltaTotal)
	if ratio < 0 {
		return 0
	}
	if ratio > 1 {
		return 1
	}
	return ratio
}
