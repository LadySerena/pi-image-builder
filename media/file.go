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

package media

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	"github.com/c2h5oh/datasize"
)

const extractName = "ubuntu-20.04.4-preinstalled-server-arm64+raspi.img"
const expectedSize = 4 * datasize.GB

type DeviceOutput struct {
	Loopdevices []Entry `json:"loopdevices"`
}

func (o DeviceOutput) ToMap() map[string]Entry {
	devices := make(map[string]Entry)
	for _, entry := range o.Loopdevices {
		devices[entry.BackFile] = entry
	}
	return devices
}

type Entry struct {
	Name      string `json:"name"`
	Sizelimit int    `json:"sizelimit"`
	Offset    int    `json:"offset"`
	Autoclear bool   `json:"autoclear"`
	Ro        bool   `json:"ro"`
	BackFile  string `json:"back-file"`
	Dio       bool   `json:"dio"`
	LogSec    int    `json:"log-sec"`
}

type PartitionEntry struct {
	Number     uint64
	Start      datasize.ByteSize
	End        datasize.ByteSize
	Size       datasize.ByteSize
	FileSystem string
}

func parsePartedOutput(output []byte) (PartitionEntry, error) {

	lines := bytes.Split(output, []byte("\n"))
	for _, line := range lines {
		split := bytes.Split(line, []byte(":"))
		// todo extract to constant
		if len(split) != 7 {
			continue
		}
		// todo extract to constant
		fileSystem := split[4]
		if bytes.Equal(fileSystem, []byte("ext4")) {
			number, conversionErr := strconv.Atoi(string(split[0]))
			if conversionErr != nil {
				return PartitionEntry{}, conversionErr
			}

			start, startErr := datasize.Parse(split[1])
			if startErr != nil {
				return PartitionEntry{}, startErr
			}

			end, endErr := datasize.Parse(split[2])
			if endErr != nil {
				return PartitionEntry{}, endErr
			}

			size, sizeErr := datasize.Parse(split[3])
			if sizeErr != nil {
				return PartitionEntry{}, sizeErr
			}

			return PartitionEntry{
				Number:     uint64(number),
				Start:      start,
				End:        end,
				Size:       size,
				FileSystem: string(fileSystem),
			}, nil
		}
	}

	return PartitionEntry{}, nil
}

func ExtractImage() (string, error) {

	_, alreadyExtracted := os.Stat(extractName)
	if alreadyExtracted == nil {
		return extractName, nil
	}

	filePath, err := filepath.Abs(ImageName)
	if err != nil {
		return "", err
	}
	_, statErr := os.Stat(filePath)
	if statErr != nil {
		return "", statErr
	}

	command := exec.Command("xz", "-d", "-k", filePath)
	return extractName, command.Run()
}

func ExpandSize() error {
	path, pathErr := filepath.Abs(extractName)
	if pathErr != nil {
		return pathErr
	}
	info, statErr := os.Stat(path)
	if statErr != nil {
		return statErr
	}

	if info.Size() > int64(expectedSize.Bytes()) {
		return nil
	}
	file, openErr := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, info.Mode())
	if openErr != nil {
		return openErr
	}
	bufferSize := datasize.MB * 1000
	newSize := int64(bufferSize.Bytes()) + info.Size()
	return file.Truncate(newSize)
}

func MountImage() (Entry, error) {

	path, pathErr := filepath.Abs(extractName)
	if pathErr != nil {
		return Entry{}, pathErr
	}
	loopCreateCommand := exec.Command("sudo", "losetup", "-Pf", path)
	loopErr := loopCreateCommand.Run()
	if loopErr != nil {
		return Entry{}, loopErr
	}

	loopDeviceCommand := exec.Command("losetup", "-lJ")
	output, pipeErr := loopDeviceCommand.StdoutPipe()
	if pipeErr != nil {
		return Entry{}, pipeErr
	}
	listErr := loopDeviceCommand.Start()
	if listErr != nil {
		return Entry{}, listErr
	}
	parsedOutput := DeviceOutput{}
	marshalErr := json.NewDecoder(output).Decode(&parsedOutput)
	if marshalErr != nil {
		return Entry{}, marshalErr
	}
	if err := loopDeviceCommand.Wait(); err != nil {
		return Entry{}, err
	}
	devices := parsedOutput.ToMap()

	return devices[path], nil

}

func FileSystemExpansion(device Entry) error {
	partitionCommand := exec.Command("sudo", "parted", device.Name, "print", "-m", "-s") //nolint:gosec
	output, pipeCreateErr := partitionCommand.StdoutPipe()
	if pipeCreateErr != nil {
		return nil
	}
	if err := partitionCommand.Start(); err != nil {
		return nil
	}
	partitions, readErr := io.ReadAll(output)
	if readErr != nil {
		return nil
	}
	if err := partitionCommand.Wait(); err != nil {
		return nil
	}
	partition, parseErr := parsePartedOutput(partitions)
	if parseErr != nil {
		return parseErr
	}

	// todo figure out how to fix resizing the partition (end is outside of the bounds of the partition)
	end := strconv.FormatUint(partition.End.Bytes(), 10)

	resizePartition := exec.Command("sudo", "parted", device.Name, "resizepart", strconv.FormatUint(partition.Number, 10), end, "-s") //nolint:gosec
	if err := resizePartition.Run(); err != nil {
		return err
	}

	partitionName := fmt.Sprintf("%sp%d", device.Name, partition.Number)

	fsCheck := exec.Command("sudo", "e2fsck", "-pf", partitionName) //nolint:gosec
	if err := fsCheck.Run(); err != nil {
		return err
	}

	resizeFS := exec.Command("sudo", "resize2fs", partitionName) //nolint:gosec
	if err := resizeFS.Run(); err != nil {
		return err
	}
	return nil
}
