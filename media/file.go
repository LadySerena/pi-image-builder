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
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/LadySerena/pi-image-builder/telemetry"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/c2h5oh/datasize"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/afero"
)

const (
	extractName         = "ubuntu-20.04.4-preinstalled-server-arm64+raspi.img"
	expectedSize        = 4 * datasize.GB
	resolvConf          = "/etc/resolv.conf"
	rootMountPoint      = "./mnt"
	bootMountPoint      = "./mnt/boot/firmware"
	mountedResolv       = "./mnt/etc/resolv.conf"
	mountedResolvBackup = "./mnt/etc/resolve.conf.bak"
)

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

func ExtractImage(ctx context.Context) (string, error) {

	_, span := telemetry.GetTracer().Start(ctx, "Extract Image")
	defer span.End()

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

func ExpandSize(ctx context.Context) error {
	_, span := telemetry.GetTracer().Start(ctx, "Expand image file")
	defer span.End()

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

func MountImageToDevice(ctx context.Context) (Entry, error) {

	_, span := telemetry.GetTracer().Start(ctx, "map image to loop device")
	defer span.End()

	path, pathErr := filepath.Abs(extractName)
	if pathErr != nil {
		return Entry{}, pathErr
	}
	loopCreateCommand := exec.Command("losetup", "-Pf", path)
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

func FileSystemExpansion(ctx context.Context, device Entry) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "expand partition and filesystem")
	defer span.End()

	partitionCommand := exec.Command("parted", "-s", "-m", device.Name, "--", "unit", "B", "print") //nolint:gosec
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

	end := fmt.Sprintf("%dB", partition.End.Bytes())

	resizePartition := exec.Command("parted", device.Name, "resizepart", strconv.FormatUint(partition.Number, 10), end, "-s") //nolint:gosec
	if err := utility.RunCommandWithOutput(ctx, resizePartition); err != nil {
		return err
	}

	partitionName := fmt.Sprintf("%sp%d", device.Name, partition.Number)

	fsCheck := exec.Command("e2fsck", "-pf", partitionName) //nolint:gosec
	if err := utility.RunCommandWithOutput(ctx, fsCheck); err != nil {
		return err
	}

	resizeFS := exec.Command("resize2fs", partitionName) //nolint:gosec
	if err := utility.RunCommandWithOutput(ctx, resizeFS); err != nil {
		return err
	}
	return nil
}

func AttachToMountPoint(ctx context.Context, fileSystem afero.Fs, device Entry) error {

	_, span := telemetry.GetTracer().Start(ctx, "mount loop device")
	defer span.End()
	if err := fileSystem.MkdirAll(bootMountPoint, 0751); err != nil {
		return err
	}

	// todo get more info about the partition layout instead of hard coding
	if err := exec.Command("mount", fmt.Sprintf("%sp2", device.Name), rootMountPoint).Run(); err != nil { //nolint:gosec
		return err
	}

	if err := exec.Command("mount", fmt.Sprintf("%sp1", device.Name), bootMountPoint).Run(); err != nil { //nolint:gosec
		return err
	}

	if err := os.Symlink("../run/systemd/resolve/stub-resolv.conf", mountedResolvBackup); err != nil {
		return err
	}

	fileInfo, err := fileSystem.Stat(resolvConf)
	if err != nil {
		return err
	}

	if err := fileSystem.Remove(mountedResolv); err != nil {
		return err
	}

	resolve, readErr := afero.ReadFile(fileSystem, resolvConf)
	if readErr != nil {
		return readErr
	}

	if err := afero.WriteFile(fileSystem, mountedResolv, resolve, fileInfo.Mode()); err != nil {
		return err
	}

	return nil
}

func CleanupAndCompress(ctx context.Context, fileSystem afero.Fs, device Entry) error {

	_, span := telemetry.GetTracer().Start(ctx, "clean up resources")
	defer span.End()

	if err := fileSystem.Remove(mountedResolv); err != nil {
		return err
	}

	if err := os.Symlink("../run/systemd/resolve/stub-resolv.conf", mountedResolv); err != nil {
		return err
	}

	if err := os.Remove(mountedResolvBackup); err != nil {
		return err
	}

	if err := exec.Command("umount", bootMountPoint).Run(); err != nil {
		return err
	}

	if err := exec.Command("umount", rootMountPoint).Run(); err != nil {
		return err
	}

	if err := exec.Command("losetup", "--detach", device.Name).Run(); err != nil { //nolint:gosec
		return err
	}

	now := time.Now()

	newImageName := fmt.Sprintf("ubuntu-20-04-arm64-%s-%d.img", now.Format("01-02-2006"), now.UnixMilli())

	if err := fileSystem.Rename(extractName, newImageName); err != nil {
		return err
	}
	file, fileErr := fileSystem.Open(newImageName)
	if fileErr != nil {
		return fileErr
	}

	defer utility.WrappedClose(file)

	compressedFile, fileOpenErr := fileSystem.Create(fmt.Sprintf("%s.zstd", newImageName))
	if fileOpenErr != nil {
		return fileOpenErr
	}
	defer utility.WrappedClose(compressedFile)

	compressor, compressorErr := zstd.NewWriter(compressedFile, zstd.WithEncoderLevel(zstd.SpeedBestCompression))
	if compressorErr != nil {
		return compressorErr
	}
	defer utility.WrappedClose(compressor)

	if _, err := io.Copy(compressor, file); err != nil {
		return err
	}

	return nil
}
