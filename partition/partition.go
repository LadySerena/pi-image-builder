/*
 * Copyright (c) 2022 Serena Tiede
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *    http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package partition

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"

	"github.com/LadySerena/pi-image-builder/utility"
)

const (
	// lvmMebibytes unit flag for mebibytes as seen in https://man.archlinux.org/man/vgs.8.en
	lvmMebibytes = "m"
	lvmBytes     = "B"
	// byteToMebibyteFactor 1024^2 to go from bytes to kibibytes to mebibytes
	byteToMebibyteFactor = 1024 * 1024
	byteToGibibyteFactor = byteToMebibyteFactor * 1024
)

type PrintOutput struct {
	Disk struct {
		Label              string `json:"label"`
		LogicalSectorSize  int    `json:"logical-sector-size"`
		MaxPartitions      int    `json:"max-partitions"`
		Model              string `json:"model"`
		Partitions         []PartitionEntry
		Path               string `json:"path"`
		PhysicalSectorSize int    `json:"physical-sector-size"`
		Size               string `json:"size"`
		Transport          string `json:"transport"`
	} `json:"disk"`
}

type PartitionEntry struct {
	End        string   `json:"end"`
	Filesystem string   `json:"filesystem"`
	Flags      []string `json:"flags"`
	Number     int      `json:"number"`
	Size       string   `json:"size"`
	Start      string   `json:"start"`
	Type       string   `json:"type"`
}

type VolumeGroupReport struct {
	Report []struct {
		VG []VolumeGroupEntry `json:"vg"`
	} `json:"report"`
}

type VolumeGroupEntry struct {
	Name        string `json:"vg_name"`
	PvCount     string `json:"pv_count"`
	LvCount     string `json:"lv_count"`
	SnapCount   string `json:"snap_count"`
	VGAttribute string `json:"vg_attr"`
	VGSize      string `json:"vg_size"`
	VGFree      string `json:"vg_free"`
}

func ToLvmArgument(s int) string {
	return fmt.Sprintf("%dB", s)
}

type SlicedVolumeGroup struct {
	RootVolumeSize int
	CSIVolumeSize  int
}

func GetPartitionTable(device string) (PrintOutput, error) {
	existing := exec.Command("parted", "-j", device, "unit", "MiB", "print")
	outputReader, pipeCreateErr := existing.StdoutPipe()
	if pipeCreateErr != nil {
		return PrintOutput{}, pipeCreateErr
	}
	if err := existing.Start(); err != nil {
		return PrintOutput{}, err
	}
	jsonBlob, readErr := io.ReadAll(outputReader)
	if readErr != nil {
		return PrintOutput{}, readErr
	}
	if err := existing.Wait(); err != nil {
		return PrintOutput{}, err
	}
	parsedOutput := PrintOutput{}
	if err := json.Unmarshal(jsonBlob, &parsedOutput); err != nil {
		return PrintOutput{}, err
	}

	return parsedOutput, nil
}

func partedCommand(device string, options ...string) *exec.Cmd {
	args := append([]string{"-s", device}, options...)
	return exec.Command("parted", args...)
}

func CreateTable(device string) error {

	currentTable, tableErr := GetPartitionTable(device)
	if tableErr != nil {
		return tableErr
	}

	if len(currentTable.Disk.Partitions) != 0 {
		return fmt.Errorf("device: %s does not have an empty partition table", device)
	}

	table := partedCommand(device, "mktable", "msdos")
	if err := table.Run(); err != nil {
		return err
	}

	boot := partedCommand(device, "mkpart", "primary", "fat32", "2048s", "257MiB")
	if err := boot.Run(); err != nil {
		return err
	}

	root := partedCommand(device, "mkpart", "primary", "ext4", "257MiB", "100%")
	if err := root.Run(); err != nil {
		return err
	}

	lvmEnable := partedCommand(device, "set", "2", "lvm", "on")
	if err := lvmEnable.Run(); err != nil {
		return err
	}

	return nil
}

func CreateLogicalVolumes(device string) error {

	rootPartition := fmt.Sprintf("%s2", device)

	physicalVolume := exec.Command("pvcreate", rootPartition)

	if err := utility.RunCommandWithOutput(context.TODO(), physicalVolume, nil); err != nil {
		return err
	}

	volumeGroup := exec.Command("vgcreate", utility.VolumeGroupName, rootPartition) //nolint:gosec
	if err := utility.RunCommandWithOutput(context.TODO(), volumeGroup, nil); err != nil {
		return err
	}

	vgReport := exec.Command("vgs", utility.VolumeGroupName, "--reportformat", "json", "--units", lvmBytes) //nolint:gosec
	output, reportErr := vgReport.CombinedOutput()
	if reportErr != nil {
		return reportErr
	}

	parsedReport := VolumeGroupReport{}
	unmarshalErr := json.Unmarshal(output, &parsedReport)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	vgSize := parsedReport.Report[0].VG[0]
	root, csi, containerd, logicalSliceErr := GetLogicalVolumeSizes(vgSize)
	if logicalSliceErr != nil {
		return logicalSliceErr
	}

	rootLogicalVolume := exec.Command("lvcreate", "--size", ToLvmArgument(root), utility.VolumeGroupName, "-n", utility.RootLogicalVolume, "--wipesignatures", "y") //nolint:gosec
	if err := utility.RunCommandWithOutput(context.TODO(), rootLogicalVolume, nil); err != nil {
		return err
	}

	csiLogicalVolume := exec.Command("lvcreate", "--size", ToLvmArgument(csi), utility.VolumeGroupName, "-n", utility.CSILogicalVolume, "--wipesignatures", "y") //nolint:gosec
	if err := utility.RunCommandWithOutput(context.TODO(), csiLogicalVolume, nil); err != nil {
		return err
	}

	containerdlogicalVolume := exec.Command("lvcreate", "--size", ToLvmArgument(containerd), utility.VolumeGroupName, "-n", utility.ContainerdVolume, "--wipesignatures", "y") //nolint:gosec
	if err := utility.RunCommandWithOutput(context.TODO(), containerdlogicalVolume, nil); err != nil {
		return err
	}

	return nil
}

func CreateFileSystems(device string) error {
	// assume sd* for device

	// TODO sort out different block devices (loop, nvme append p$NUM) others just have the number at the end
	bootPartition := fmt.Sprintf("%s1", device)

	bootFS := exec.Command("mkfs.vfat", "-F", "32", "-n", "system-boot", bootPartition)
	if err := bootFS.Run(); err != nil {
		return err
	}

	rootFS := exec.Command("mkfs.ext4", utility.MapperName(utility.RootLogicalVolume)) //nolint:gosec
	if err := rootFS.Run(); err != nil {
		return err
	}

	csiFS := exec.Command("mkfs.ext4", utility.MapperName(utility.CSILogicalVolume)) //nolint:gosec
	if err := csiFS.Run(); err != nil {
		return err
	}

	containerdFS := exec.Command("mkfs.ext4", utility.MapperName(utility.ContainerdVolume)) //nolint:gosec
	if err := containerdFS.Run(); err != nil {
		return err
	}

	return nil
}

func GetLogicalVolumeSizes(entry VolumeGroupEntry) (rootSize int, CSISize int, containerdSize int, err error) {

	initialSize := entry.VGFree
	initialSize = strings.TrimSuffix(initialSize, lvmBytes)
	parsedSize, conversionErr := strconv.Atoi(initialSize)
	if conversionErr != nil {
		return rootSize, CSISize, containerdSize, conversionErr
	}

	availableSize := parsedSize - (2 * 256 * byteToMebibyteFactor)

	rootSize = 10 * byteToGibibyteFactor

	containerdSize = 30 * byteToGibibyteFactor

	CSISize = availableSize - rootSize - containerdSize

	if CSISize < (5 * byteToGibibyteFactor) {
		return rootSize, CSISize, containerdSize, fmt.Errorf("volumegroups: %s does not have enough capacity for csi storage", entry.Name)
	}

	return rootSize, CSISize, containerdSize, nil
}
