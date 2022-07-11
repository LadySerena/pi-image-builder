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
	"os"
	"os/exec"
	"path/filepath"

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
	Type       string
	FileSystem string
	Flags      *string
}

// todo finish parsing this mess
//$ sudo parted /dev/loop5 print -m -s
//BYT;
///dev/loop5:4536MB:loopback:512:512:msdos:Loopback device:;
//1:1049kB:269MB:268MB:fat32::boot, lba;
//2:269MB:3488MB:3218MB:ext4::;
//(⎈ |serena@kubernetes:default)serena@serena-desktop:~/repos/pi-image-builder ‹serena/ubuntu›
//$ sudo parted /dev/loop5 print
//Model: Loopback device (loopback)
//Disk /dev/loop5: 4536MB
//Sector size (logical/physical): 512B/512B
//Partition Table: msdos
//Disk Flags:
//
//Number  Start   End     Size    Type     File system  Flags
//1      1049kB  269MB   268MB   primary  fat32        boot, lba
//2      269MB   3488MB  3218MB  primary  ext4

func parsePartedOutput(output []byte) PartitionEntry {
	lines := bytes.Split(output, []byte("\n"))

	return PartitionEntry{}
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
	loopCreateCommand := exec.Command("sudo", "losetup", "-Pf", extractName)
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

	return devices[extractName], nil

}

func FileSystemExpansion(device Entry) {
	partitionCommand := exec.Command("sudo", "parted", device.Name, "print", "-m", "-s")
	output, pipeCreateErr := partitionCommand.StdoutPipe()
	if pipeCreateErr != nil {
		return
	}

}
