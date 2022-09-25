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

	"github.com/LadySerena/pi-image-builder/utility"
)

type PrintOutput struct {
	Disk struct {
		Label             string `json:"label"`
		LogicalSectorSize int    `json:"logical-sector-size"`
		MaxPartitions     int    `json:"max-partitions"`
		Model             string `json:"model"`
		Partitions        []struct {
			End        string   `json:"end"`
			Filesystem string   `json:"filesystem"`
			Flags      []string `json:"flags"`
			Number     int      `json:"number"`
			Size       string   `json:"size"`
			Start      string   `json:"start"`
			Type       string   `json:"type"`
		} `json:"partitions,omitempty"`
		Path               string `json:"path"`
		PhysicalSectorSize int    `json:"physical-sector-size"`
		Size               string `json:"size"`
		Transport          string `json:"transport"`
	} `json:"disk"`
}

func verifyEmptyPartitionTable(device string) error {
	existing := exec.Command("parted", "-j", device, "print")
	outputReader, pipeCreateErr := existing.StdoutPipe()
	if pipeCreateErr != nil {
		return pipeCreateErr
	}
	if err := existing.Start(); err != nil {
		return err
	}
	jsonBlob, readErr := io.ReadAll(outputReader)
	if readErr != nil {
		return readErr
	}
	if err := existing.Wait(); err != nil {
		return err
	}
	parsedOutput := PrintOutput{}
	if err := json.Unmarshal(jsonBlob, &parsedOutput); err != nil {
		return err
	}

	if len(parsedOutput.Disk.Partitions) != 0 {
		return fmt.Errorf("device: %s does not have an empty partition table", device)
	}
	return nil
}

func partedCommand(device string, options ...string) *exec.Cmd {
	args := append([]string{"-s", device}, options...)
	return exec.Command("parted", args...)
}

func CreateTable(device string) error {

	if err := verifyEmptyPartitionTable(device); err != nil {
		return err
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
	// TODO dynamically figure out how to leave about 256MiB for scrubs
	// ideally this will be equal to totalSize - 256MiB aka 64 extents
	size := "7410"

	physicalVolume := exec.Command("pvcreate", rootPartition)

	if err := utility.RunCommandWithOutput(context.TODO(), physicalVolume, nil); err != nil {
		return err
	}

	volumeGroup := exec.Command("vgcreate", utility.VolumeGroupName, rootPartition)
	if err := utility.RunCommandWithOutput(context.TODO(), volumeGroup, nil); err != nil {
		return err
	}

	logicalVolume := exec.Command("lvcreate", "--extents", size, utility.VolumeGroupName, "-n", utility.LogicalVolumeName, "--wipesignatures", "y")
	if err := utility.RunCommandWithOutput(context.TODO(), logicalVolume, nil); err != nil {
		return err
	}

	return nil
}

func CreateFileSystems(device string) error {
	// assume sd* for device
	mapperName := utility.MapperName()

	// TODO sort out different block devices (loop, nvme append p$NUM) others just have the number at the end
	bootPartition := fmt.Sprintf("%s1", device)

	bootFS := exec.Command("mkfs.vfat", "-F", "32", "-n", "system-boot", bootPartition)
	if err := bootFS.Run(); err != nil {
		return err
	}

	rootFS := exec.Command("mkfs.ext4", mapperName)
	if err := rootFS.Run(); err != nil {
		return err
	}

	return nil
}
