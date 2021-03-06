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
	"time"

	"github.com/LadySerena/pi-image-builder/image"
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
	err := media.DownloadAndVerifyMedia(localFS, false)
	if err != nil {
		log.Fatalf("error with downloading media: %v", err)
	}
	allocateErr := image.AllocateFile(time.Now())
	if allocateErr != nil {
		log.Fatalf("error with allocating file: %v", allocateErr)
	}
}
