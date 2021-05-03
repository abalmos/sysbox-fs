//
// Copyright 2019-2020 Nestybox, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//    https://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
//

package implementations

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/sirupsen/logrus"

	"github.com/nestybox/sysbox-fs/domain"
)

// FIXME:
//
// This is a base handler for kernel sysctls exposed inside a sys container that
// consist of a single integer value and where the value written to the host
// kernel is the max value across sys containers.

type ProcSysNetIpv4Neigh struct {
	domain.HandlerBase
}

var ProcSysNetIpv4Neigh_Handler = &ProcSysNetIpv4Neigh{
	domain.HandlerBase{
		Name: "ProcSysNetIpv4Neigh",
		Path: "/proc/sys/net/ipv4/neigh",
		EmuNodesMap: map[string]domain.EmuNode{
			"default":            domain.EmuNode{domain.EmuNodeDir, os.FileMode(uint32(0555))},
			"default/gc_thresh1": domain.EmuNode{domain.EmuNodeFile, os.FileMode(uint32(0644))},
			"default/gc_thresh2": domain.EmuNode{domain.EmuNodeFile, os.FileMode(uint32(0644))},
			"default/gc_thresh3": domain.EmuNode{domain.EmuNodeFile, os.FileMode(uint32(0644))},
		},
		Type:      domain.NODE_SUBSTITUTION,
		Enabled:   true,
		Cacheable: true,
	},
}

func (h *ProcSysNetIpv4Neigh) Lookup(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) (os.FileInfo, error) {

	logrus.Debugf("Executing Lookup() method on %v handler", h.Name)

	// Obtain relative path to the element being looked up.
	relPath, err := filepath.Rel(h.Path, n.Path())
	if err != nil {
		return nil, err
	}

	var lookupEntry string

	// Adjust the lookup-ed element to match the virtual-component's
	// representation convention.
	relPathDir := filepath.Dir(relPath)
	if relPathDir == "." ||
		strings.HasPrefix(relPath, "default/gc_thresh") {
		lookupEntry = relPath
	}

	// Return an artificial fileInfo if looked-up element matches any of the
	// virtual-components.
	if v, ok := h.EmuNodesMap[lookupEntry]; ok {
		info := &domain.FileInfo{
			Fname:    lookupEntry,
			FmodTime: time.Now(),
		}

		if v.Kind == domain.EmuNodeDir {
			info.Fmode = os.FileMode(uint32(os.ModeDir)) | v.Mode
			info.FisDir = true
		} else if v.Kind == domain.EmuNodeFile {
			info.Fmode = v.Mode
		}

		return info, nil
	}

	// If looked-up element hasn't been found by now, let's look into the actual
	// sys container rootfs.
	procSysCommonHandler, ok := h.Service.FindHandler("/proc/sys/")
	if !ok {
		return nil, fmt.Errorf("No /proc/sys/ handler found")
	}

	return procSysCommonHandler.Lookup(n, req)
}

func (h *ProcSysNetIpv4Neigh) Getattr(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) (*syscall.Stat_t, error) {

	logrus.Debugf("Executing Getattr() method for Req ID=%#x on %v handler", req.ID, h.Name)

	// Ensure operation is generated from within a registered sys container.
	if req.Container == nil {
		logrus.Errorf("Could not find the container originating this request (pid %v)",
			req.Pid)
		return nil, errors.New("Container not found")
	}

	stat := &syscall.Stat_t{
		Uid: req.Container.UID(),
		Gid: req.Container.GID(),
	}

	return stat, nil
}

func (h *ProcSysNetIpv4Neigh) Open(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) error {

	return nil
}

func (h *ProcSysNetIpv4Neigh) Close(n domain.IOnodeIface) error {

	return nil
}

func (h *ProcSysNetIpv4Neigh) Read(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) (int, error) {

	logrus.Debugf("Executing Read() method for Req ID=%#x on %v handler",
		req.ID, h.Name)

	// We are dealing with a single integer element being read, so we can save
	// some cycles by returning right away if offset is any higher than zero.
	if req.Offset > 0 {
		return 0, io.EOF
	}

	cntr := req.Container

	// Ensure operation is generated from within a registered sys container.
	if cntr == nil {
		logrus.Errorf("Could not find the container originating this request (pid %v)",
			req.Pid)
		return 0, errors.New("Container not found")
	}

	// As the "neighbor" node isn't exposed within containers, sysbox's integration
	// testsuites will fail when executing within the test framework. In these cases,
	// we will redirect all "neighbor" queries to a static node that is always present
	// in the testing environment.
	if h.GetService().IgnoreErrors() {
		n.SetPath("/proc/sys/net/ipv4/neigh/lo/retrans_time")
	}

	return readFileInt(h, n, req)
}

// FIXME: We should write the max default vals down to kernel.
//
//
func (h *ProcSysNetIpv4Neigh) Write(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) (int, error) {

	logrus.Debugf("Executing %v Write() method", h.Name)

	cntr := req.Container

	newVal := strings.TrimSpace(string(req.Data))
	_, err := strconv.Atoi(newVal)
	if err != nil {
		logrus.Errorf("Unexpected error: %v", err)
		return 0, err
	}

	// Ensure operation is generated from within a registered sys container.
	if cntr == nil {
		logrus.Errorf("Could not find the container originating this request (pid %v)",
			req.Pid)
		return 0, errors.New("Container not found")
	}

	// As the "neighbor" node isn't exposed within containers, sysbox's integration
	// testsuites will fail when executing within the test framework. In these cases,
	// we will redirect all "neighbor" queries to a static node that is always present
	// in the testing environment.
	if h.GetService().IgnoreErrors() {
		n.SetPath("/proc/sys/net/ipv4/neigh/lo/retrans_time")
	}

	return writeInt(h, n, req, MinInt, MaxInt, false)
}

func (h *ProcSysNetIpv4Neigh) ReadDirAll(
	n domain.IOnodeIface,
	req *domain.HandlerRequest) ([]os.FileInfo, error) {

	logrus.Debugf("Executing ReadDirAll() method for Req ID=%#x on %v handler",
		req.ID, h.Name)

	// Ensure operation is generated from within a registered sys container.
	if req.Container == nil {
		logrus.Errorf("Could not find the container originating this request (pid %v)",
			req.Pid)
		return nil, errors.New("Container not found")
	}

	var (
		info        *domain.FileInfo
		fileEntries []os.FileInfo
	)

	// Obtain relative path to the element being read.
	relpath, err := filepath.Rel(h.Path, n.Path())
	if err != nil {
		return nil, err
	}

	// Iterate through map of virtual components.
	for k, _ := range h.EmuNodesMap {

		if relpath == filepath.Dir(k) {
			info = &domain.FileInfo{
				Fname:    filepath.Base(k),
				Fmode:    os.ModeDir,
				FmodTime: time.Now(),
				FisDir:   true,
			}

			fileEntries = append(fileEntries, info)

		} else if relpath != "." && relpath == filepath.Dir(k) {
			info = &domain.FileInfo{
				Fname:    filepath.Base(k),
				FmodTime: time.Now(),
			}

			fileEntries = append(fileEntries, info)
		}
	}

	// Also collect procfs entries as seen within container's namespaces.
	procSysCommonHandler, ok := h.Service.FindHandler("/proc/sys/")
	if !ok {
		return nil, fmt.Errorf("No /proc/sys/ handler found")
	}
	commonNeigh, err := procSysCommonHandler.ReadDirAll(n, req)
	if err == nil {
		for _, entry := range commonNeigh {
			fileEntries = append(fileEntries, entry)
		}
	}

	return fileEntries, nil
}

func (h *ProcSysNetIpv4Neigh) GetName() string {
	return h.Name
}

func (h *ProcSysNetIpv4Neigh) GetPath() string {
	return h.Path
}

func (h *ProcSysNetIpv4Neigh) GetEnabled() bool {
	return h.Enabled
}

func (h *ProcSysNetIpv4Neigh) GetType() domain.HandlerType {
	return h.Type
}

func (h *ProcSysNetIpv4Neigh) GetService() domain.HandlerServiceIface {
	return h.Service
}

func (h *ProcSysNetIpv4Neigh) GetMutex() sync.Mutex {
	return h.Mutex
}

func (h *ProcSysNetIpv4Neigh) SetEnabled(val bool) {
	h.Enabled = val
}

func (h *ProcSysNetIpv4Neigh) SetService(hs domain.HandlerServiceIface) {
	h.Service = hs
}