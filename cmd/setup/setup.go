/*
 * Copyright (c) 2021 Serena Tiede
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

package main

import (
	"log"

	"github.com/LadySerena/pi-image-builder/configure"
	"github.com/LadySerena/pi-image-builder/media"
	"github.com/spf13/afero"
)

// steps
// * grab install media
// * verify checksum
// * allocate file
// setup loop device
// mount file on loop device
// partition device
// create filesystems
// mount loop devices
// decompress onto mount point
// configure temp dns
// copy binfmt files (if on x86)
// nspawn into mount
// do configuration
// remove binfmt files
// undo dns changes

func main() {
	localFS := afero.NewOsFs()
	mountedFs := afero.NewBasePathFs(localFS, "./mnt")

	if err := media.DownloadAndVerifyMedia(localFS, false); err != nil {
		log.Fatalf("error with downloading media: %v", err)
	}

	_, decompressErr := media.ExtractImage()
	if decompressErr != nil {
		log.Fatalf("error decompressing image: %s", decompressErr)
	}
	truncateErr := media.ExpandSize()
	if truncateErr != nil {
		log.Fatalf("error expanding image size: %s", truncateErr)
	}

	device, mountFileErr := media.MountImageToDevice()
	if mountFileErr != nil {
		log.Fatalf("error mounting image: %s", mountFileErr)
	}

	defer func(fileSystem afero.Fs, device media.Entry) {
		err := media.Cleanup(fileSystem, device)
		if err != nil {
			log.Fatalf("error cleaning up resources: %v", err)
		}
	}(localFS, device)

	if err := media.FileSystemExpansion(device); err != nil {
		log.Panicf("error expanding file system: %v", err)
	}

	if err := media.AttachToMountPoint(localFS, device); err != nil {
		log.Panicf("error mounting image: %v", err)
	}

	if err := configure.KernelSettings(mountedFs); err != nil {
		log.Panicf("error configuring kernel settings: %v", err)
	}

	if err := configure.KernelModules(mountedFs); err != nil {
		log.Panicf("error configuring modules and sysctls: %v", err)
	}

	if err := configure.Packages(mountedFs); err != nil {
		log.Panicf("error installing packages: %v", err)
	}

	if err := configure.InstallKubernetes(mountedFs, "v1.24.3", "v1.24.2", "v1.1.1", "v0.4.0"); err != nil {
		log.Panicf("error installing Kubernetes: %s", err)
	}
}
