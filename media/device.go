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
	"context"
	"fmt"
	"os/exec"

	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
)

const (
	mediaRoot = "./media-mnt"
	mediaBoot = "./media-mnt/boot/firmware"
)

func MountMedia(ctx context.Context, fileSystem afero.Fs, device string) error {

	if err := fileSystem.MkdirAll(mediaRoot, 0751); err != nil {
		return err
	}

	if err := exec.Command("mount", utility.MapperName(""), mediaRoot).Run(); err != nil { //nolint:gosec
		return err
	}

	if err := fileSystem.MkdirAll(mediaBoot, 0751); err != nil {
		return err
	}

	if err := exec.Command("mount", fmt.Sprintf("%s1", device), mediaBoot).Run(); err != nil { //nolint:gosec
		return err
	}

	return nil
}

func Flash(ctx context.Context, device string, entry Entry) error {
	bootSync := exec.Command("rsync", "--progress", "-axv", utility.TrailingSlash(bootMountPoint), utility.TrailingSlash(mediaBoot)) //nolint:gosec
	rootSync := exec.Command("rsync", "--progress", "-axv", utility.TrailingSlash(rootMountPoint), utility.TrailingSlash(mediaRoot)) //nolint:gosec

	if err := utility.RunCommandWithOutput(ctx, bootSync, nil); err != nil {
		return err
	}

	return utility.RunCommandWithOutput(ctx, rootSync, nil)
}
