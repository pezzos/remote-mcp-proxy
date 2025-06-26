package monitoring

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"remote-mcp-proxy/logger"
)

type ProcessMetrics struct {
	PID        int       `json:"pid"`
	Name       string    `json:"name"`
	MemoryMB   float64   `json:"memoryMB"`
	CPUPercent float64   `json:"cpuPercent"`
	VirtualMB  float64   `json:"virtualMB"`
	ResidentMB float64   `json:"residentMB"`
	Timestamp  time.Time `json:"timestamp"`
}

type ResourceMonitor struct {
	logger          *logger.Logger
	interval        time.Duration
	stopChan        chan bool
	alertThresholds map[string]float64
}

func NewResourceMonitor() *ResourceMonitor {
	return &ResourceMonitor{
		logger:   logger.System(),
		interval: 60 * time.Second, // Monitor every minute
		stopChan: make(chan bool),
		alertThresholds: map[string]float64{
			"memory_mb":   500, // Alert if any process uses > 500MB
			"cpu_percent": 80,  // Alert if any process uses > 80% CPU
		},
	}
}

func (rm *ResourceMonitor) Start() {
	rm.logger.Info("Starting resource monitor (interval: %v)", rm.interval)

	go func() {
		ticker := time.NewTicker(rm.interval)
		defer ticker.Stop()

		// Initial check
		rm.checkResources()

		for {
			select {
			case <-ticker.C:
				rm.checkResources()
			case <-rm.stopChan:
				rm.logger.Info("Resource monitor stopped")
				return
			}
		}
	}()
}

func (rm *ResourceMonitor) Stop() {
	close(rm.stopChan)
}

func (rm *ResourceMonitor) checkResources() {
	// Get all processes and filter for MCP-related ones
	processes, err := rm.getMCPProcesses()
	if err != nil {
		rm.logger.Error("Failed to get process metrics: %v", err)
		return
	}

	if len(processes) == 0 {
		rm.logger.Debug("No MCP processes found during resource check")
		return
	}

	// Log metrics and check thresholds
	totalMemory := 0.0
	totalCPU := 0.0

	for _, proc := range processes {
		rm.logger.Debug("Process %s (PID %d): Memory=%.1fMB, CPU=%.1f%%, Virtual=%.1fMB, Resident=%.1fMB",
			proc.Name, proc.PID, proc.MemoryMB, proc.CPUPercent, proc.VirtualMB, proc.ResidentMB)

		totalMemory += proc.MemoryMB
		totalCPU += proc.CPUPercent

		// Check individual process thresholds
		if proc.MemoryMB > rm.alertThresholds["memory_mb"] {
			rm.logger.Warn("High memory usage for process %s (PID %d): %.1fMB",
				proc.Name, proc.PID, proc.MemoryMB)
		}

		if proc.CPUPercent > rm.alertThresholds["cpu_percent"] {
			rm.logger.Warn("High CPU usage for process %s (PID %d): %.1f%%",
				proc.Name, proc.PID, proc.CPUPercent)
		}
	}

	rm.logger.Info("Resource summary: %d MCP processes, Total Memory=%.1fMB, Total CPU=%.1f%%",
		len(processes), totalMemory, totalCPU)
}

func (rm *ResourceMonitor) getMCPProcesses() ([]ProcessMetrics, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Use ps to get process information
	cmd := exec.CommandContext(ctx, "ps", "aux")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to execute ps command: %w", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return nil, fmt.Errorf("invalid ps output")
	}

	var processes []ProcessMetrics

	// Skip header line
	for _, line := range lines[1:] {
		if strings.TrimSpace(line) == "" {
			continue
		}

		// Check if this is an MCP-related process
		if !rm.isMCPProcess(line) {
			continue
		}

		proc, err := rm.parseProcessLine(line)
		if err != nil {
			rm.logger.Debug("Failed to parse process line: %v", err)
			continue
		}

		processes = append(processes, proc)
	}

	return processes, nil
}

func (rm *ResourceMonitor) isMCPProcess(line string) bool {
	mcpKeywords := []string{
		"mcp-server",
		"@modelcontextprotocol",
		"@notionhq/notion-mcp-server",
		"sequential-thinking",
		"server-memory",
		"server-filesystem",
	}

	for _, keyword := range mcpKeywords {
		if strings.Contains(line, keyword) {
			return true
		}
	}

	return false
}

func (rm *ResourceMonitor) parseProcessLine(line string) (ProcessMetrics, error) {
	// ps aux format: USER PID %CPU %MEM VSZ RSS TTY STAT START TIME COMMAND
	fields := strings.Fields(line)
	if len(fields) < 11 {
		return ProcessMetrics{}, fmt.Errorf("insufficient fields in ps output")
	}

	pid, err := strconv.Atoi(fields[1])
	if err != nil {
		return ProcessMetrics{}, fmt.Errorf("invalid PID: %w", err)
	}

	cpuPercent, err := strconv.ParseFloat(fields[2], 64)
	if err != nil {
		return ProcessMetrics{}, fmt.Errorf("invalid CPU percent: %w", err)
	}

	memPercent, err := strconv.ParseFloat(fields[3], 64)
	if err != nil {
		return ProcessMetrics{}, fmt.Errorf("invalid memory percent: %w", err)
	}

	vszKB, err := strconv.ParseFloat(fields[4], 64)
	if err != nil {
		return ProcessMetrics{}, fmt.Errorf("invalid VSZ: %w", err)
	}

	rssKB, err := strconv.ParseFloat(fields[5], 64)
	if err != nil {
		return ProcessMetrics{}, fmt.Errorf("invalid RSS: %w", err)
	}

	// Extract process name from command (last field)
	command := strings.Join(fields[10:], " ")
	name := rm.extractProcessName(command)

	// Get total system memory to calculate actual memory usage
	totalMemoryKB, err := rm.getTotalSystemMemory()
	if err != nil {
		rm.logger.Debug("Failed to get total system memory, using RSS for memory calculation")
		totalMemoryKB = rssKB * 100 / memPercent // Rough estimate
	}

	memoryMB := (memPercent * totalMemoryKB) / (100 * 1024) // Convert to MB

	return ProcessMetrics{
		PID:        pid,
		Name:       name,
		MemoryMB:   memoryMB,
		CPUPercent: cpuPercent,
		VirtualMB:  vszKB / 1024, // Convert KB to MB
		ResidentMB: rssKB / 1024, // Convert KB to MB
		Timestamp:  time.Now(),
	}, nil
}

func (rm *ResourceMonitor) extractProcessName(command string) string {
	// Extract meaningful name from command line
	if strings.Contains(command, "mcp-server-memory") {
		return "memory-server"
	}
	if strings.Contains(command, "mcp-server-filesystem") {
		return "filesystem-server"
	}
	if strings.Contains(command, "sequential-thinking") {
		return "sequential-thinking-server"
	}
	if strings.Contains(command, "notion-mcp-server") {
		return "notion-server"
	}
	if strings.Contains(command, "npm exec") {
		// Extract package name from npm exec command
		parts := strings.Fields(command)
		for i, part := range parts {
			if part == "exec" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}

	// Default to first word of command
	fields := strings.Fields(command)
	if len(fields) > 0 {
		return fields[0]
	}

	return "unknown"
}

func (rm *ResourceMonitor) getTotalSystemMemory() (float64, error) {
	// Read /proc/meminfo to get total system memory
	data, err := os.ReadFile("/proc/meminfo")
	if err != nil {
		return 0, err
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "MemTotal:") {
			fields := strings.Fields(line)
			if len(fields) >= 2 {
				memKB, err := strconv.ParseFloat(fields[1], 64)
				if err != nil {
					return 0, err
				}
				return memKB, nil
			}
		}
	}

	return 0, fmt.Errorf("MemTotal not found in /proc/meminfo")
}

func (rm *ResourceMonitor) GetCurrentMetrics() ([]ProcessMetrics, error) {
	return rm.getMCPProcesses()
}
