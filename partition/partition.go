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
	"encoding/json"
	"fmt"
	"io"
	"os/exec"

	"github.com/c2h5oh/datasize"
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

	if parsedOutput.Disk.Partitions != nil {
		return fmt.Errorf("device: %s does not have an empty partition table", device)
	}
	return nil
}

func partedCommand(device string, options ...string) *exec.Cmd {
	args := append([]string{"-s", device}, options...)
	return exec.Command("parted", args...)
}

func Create(bootSize datasize.ByteSize, device string) error {

	if err := verifyEmptyPartitionTable(device); err != nil {
		return err
	}
	//table := partedCommand(device, "mktable", "msdos")

	return nil
}
