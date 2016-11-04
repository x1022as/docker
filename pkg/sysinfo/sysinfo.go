package sysinfo

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/docker/docker/pkg/parsers"
	"github.com/docker/go-units"
)

// SysInfo stores information about which features a kernel supports.
// TODO Windows: Factor out platform specific capabilities.
type SysInfo struct {
	// Whether the kernel supports AppArmor or not
	AppArmor bool
	// Whether the kernel supports Seccomp or not
	Seccomp bool

	cgroupMemInfo
	cgroupHugetlbInfo
	cgroupCPUInfo
	cgroupBlkioInfo
	cgroupCpusetInfo
	cgroupPids

	// Whether IPv4 forwarding is supported or not, if this was disabled, networking will not work
	IPv4ForwardingDisabled bool

	// Whether bridge-nf-call-iptables is supported or not
	BridgeNFCallIPTablesDisabled bool

	// Whether bridge-nf-call-ip6tables is supported or not
	BridgeNFCallIP6TablesDisabled bool

	// Whether the cgroup has the mountpoint of "devices" or not
	CgroupDevicesEnabled bool
}

type cgroupMemInfo struct {
	// Whether memory limit is supported or not
	MemoryLimit bool

	// Whether swap limit is supported or not
	SwapLimit bool

	// Whether soft limit is supported or not
	MemoryReservation bool

	// Whether OOM killer disable is supported or not
	OomKillDisable bool

	// Whether memory swappiness is supported or not
	MemorySwappiness bool

	// Whether kernel memory limit is supported or not
	KernelMemory bool
}

type cgroupHugetlbInfo struct {
	// Whether hugetlb limit is supported or not
	HugetlbLimit bool
}

type cgroupCPUInfo struct {
	// Whether CPU shares is supported or not
	CPUShares bool

	// Whether CPU CFS(Completely Fair Scheduler) period is supported or not
	CPUCfsPeriod bool

	// Whether CPU CFS(Completely Fair Scheduler) quota is supported or not
	CPUCfsQuota bool

	// Whether CPU real-time period is supported or not
	CPURealtimePeriod bool

	// Whether CPU real-time runtime is supported or not
	CPURealtimeRuntime bool
}

type cgroupBlkioInfo struct {
	// Whether Block IO weight is supported or not
	BlkioWeight bool

	// Whether Block IO weight_device is supported or not
	BlkioWeightDevice bool

	// Whether Block IO read limit in bytes per second is supported or not
	BlkioReadBpsDevice bool

	// Whether Block IO write limit in bytes per second is supported or not
	BlkioWriteBpsDevice bool

	// Whether Block IO read limit in IO per second is supported or not
	BlkioReadIOpsDevice bool

	// Whether Block IO write limit in IO per second is supported or not
	BlkioWriteIOpsDevice bool
}

type cgroupCpusetInfo struct {
	// Whether Cpuset is supported or not
	Cpuset bool

	// Available Cpuset's cpus
	Cpus string

	// Available Cpuset's memory nodes
	Mems string
}

type cgroupPids struct {
	// Whether Pids Limit is supported or not
	PidsLimit bool
}

// ValidateHugetlb check whether hugetlb pagesize and limit legal
func (c cgroupHugetlbInfo) ValidateHugetlb(pageSize string, limit uint64) (string, []string, error) {
	var (
		w   []string
		err error
	)
	if pageSize != "" {
		sizeInt, _ := units.RAMInBytes(pageSize)
		pageSize = humanSize(sizeInt)
		if err = isHugepageSizeValid(pageSize); err != nil {
			return "", w, err
		}
	} else {
		pageSize, err = c.GetDefaultHugepageSize()
		if err != nil {
			return "", w, fmt.Errorf("Failed to get system hugepage size")
		}
	}

	warning, err := isHugeLimitValid(pageSize, limit)
	w = append(w, warning...)
	if err != nil {
		return "", w, err
	}

	return pageSize, w, nil
}

// isHugeLimitValid check whether input hugetlb limit legal
// it will check whether the limit size is times of size
func isHugeLimitValid(size string, limit uint64) ([]string, error) {
	var w []string
	sizeInt, err := units.RAMInBytes(size)
	if err != nil || sizeInt < 0 {
		return w, fmt.Errorf("Invalid hugepage size:%s -- %s", size, err)
	}
	sizeUint := uint64(sizeInt)

	if limit%sizeUint != 0 {
		w = append(w, "Invalid hugetlb limit: should be times of huge page size"+
			"cgroup will down round to the nearest multiple")
	}

	return w, nil
}

// isHugepageSizeValid check whether input size legal
// it will compare size with all system supported hugepage size
func isHugepageSizeValid(size string) error {
	hps, err := getHugepageSizes()
	if err != nil {
		return err
	}

	for _, hp := range hps {
		if size == hp {
			return nil
		}
	}
	return fmt.Errorf("Invalid hugepage size:%s, shoud be one of %v", size, hps)
}

func humanSize(i int64) string {
	// hugetlb may not surpass GB
	uf := []string{"B", "KB", "MB", "GB"}
	ui := 0
	for {
		if i < 1024 || ui >= 3 {
			break
		}
		i = int64(i / 1024)
		ui = ui + 1
	}

	return fmt.Sprintf("%d%s", i, uf[ui])
}

func getHugepageSizes() ([]string, error) {
	var hps []string

	cgMounts, err := findCgroupMountpoints()
	if err != nil {
		return nil, err
	}
	hgtlbMp, ok := cgMounts["hugetlb"]
	if !ok {
		return nil, fmt.Errorf("Hugetlb cgroup not supported")
	}

	f, err := os.Open(hgtlbMp)
	if err != nil {
		return nil, fmt.Errorf("Failed to open hugetlb cgroup directory")
	}
	// -1 here means to read all the fileInfo from the directory, could be any negative number
	fi, err := f.Readdir(-1)
	if err != nil {
		return nil, fmt.Errorf("Failed to read hugetlb cgroup directory")
	}

	for _, finfo := range fi {
		if strings.Contains(finfo.Name(), "limit_in_bytes") {
			sres := strings.SplitN(finfo.Name(), ".", 3)
			if len(sres) != 3 {
				continue
			}
			hps = append(hps, sres[1])
		}
	}
	if len(hps) == 0 {
		return nil, fmt.Errorf("Hugetlb pagesize not found in cgroup")
	}

	return hps, nil
}

// GetDefaultHugepageSize returns system default hugepage size
func (c cgroupHugetlbInfo) GetDefaultHugepageSize() (string, error) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return "", fmt.Errorf("Failed to get hugepage size, cannot open /proc/meminfo")
	}
	defer f.Close()

	s := bufio.NewScanner(f)
	for s.Scan() {
		if strings.Contains(s.Text(), "Hugepagesize") {
			sres := strings.SplitN(s.Text(), ":", 2)
			if len(sres) != 2 {
				return "", fmt.Errorf("Failed to get hugepage size, weird /proc/meminfo format")
			}

			// return strings.TrimSpace(sres[1]), nil
			size := strings.Replace(sres[1], " ", "", -1)
			// transform 2048k to 2M
			sizeInt, _ := units.RAMInBytes(size)
			return humanSize(sizeInt), nil
		}
	}
	return "", fmt.Errorf("Failed to get hugepage size")
}

// IsCpusetCpusAvailable returns `true` if the provided string set is contained
// in cgroup's cpuset.cpus set, `false` otherwise.
// If error is not nil a parsing error occurred.
func (c cgroupCpusetInfo) IsCpusetCpusAvailable(provided string) (bool, error) {
	return isCpusetListAvailable(provided, c.Cpus)
}

// IsCpusetMemsAvailable returns `true` if the provided string set is contained
// in cgroup's cpuset.mems set, `false` otherwise.
// If error is not nil a parsing error occurred.
func (c cgroupCpusetInfo) IsCpusetMemsAvailable(provided string) (bool, error) {
	return isCpusetListAvailable(provided, c.Mems)
}

func isCpusetListAvailable(provided, available string) (bool, error) {
	parsedProvided, err := parsers.ParseUintList(provided)
	if err != nil {
		return false, err
	}
	parsedAvailable, err := parsers.ParseUintList(available)
	if err != nil {
		return false, err
	}
	for k := range parsedProvided {
		if !parsedAvailable[k] {
			return false, nil
		}
	}
	return true, nil
}

// Returns bit count of 1, used by NumCPU
func popcnt(x uint64) (n byte) {
	x -= (x >> 1) & 0x5555555555555555
	x = (x>>2)&0x3333333333333333 + x&0x3333333333333333
	x += x >> 4
	x &= 0x0f0f0f0f0f0f0f0f
	x *= 0x0101010101010101
	return byte(x >> 56)
}
