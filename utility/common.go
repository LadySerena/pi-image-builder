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

package utility

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"io/fs"
	"log"
	"os/exec"
	"path"
	"strings"
	"text/template"

	"github.com/LadySerena/pi-image-builder/telemetry"
)

const (
	ExtractName       = "ubuntu-20.04.5-preinstalled-server-arm64+raspi.img"
	ImageName         = "ubuntu-20.04.5-preinstalled-server-arm64+raspi.img.xz"
	BucketName        = "pi-images.serenacodes.com"
	VolumeGroupName   = "rootvg"
	RootLogicalVolume = "rootlv"
	CSILogicalVolume  = "csilv"
	ContainerdVolume  = "containerdlv"
)

func WrappedClose(closer io.Closer) {
	if err := closer.Close(); err != nil {
		log.Panicf("could not close closer properly: %v", err)
	}
}

func RunCommandWithOutput(ctx context.Context, cmd *exec.Cmd, cancel context.CancelFunc) error {

	_, span := telemetry.GetTracer().Start(ctx, fmt.Sprintf("running command: %s", cmd.String()))
	defer span.End()
	if cancel != nil {
		defer cancel()
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("non zero exit code exit code: %v, output: %s", err, string(output))
	}

	return nil
}

func MapperName(volumeName string) string {
	return fmt.Sprintf("/dev/mapper/%s-%s", VolumeGroupName, volumeName)
}

func TrailingSlash(inputPath string) string {
	if strings.HasSuffix(inputPath, "/") {
		return inputPath
	}
	return fmt.Sprintf("%s/", inputPath)
}

func RenderTemplate(ctx context.Context, fs fs.FS, templatePath string, data any) (bytes.Buffer, error) {

	_, span := telemetry.GetTracer().Start(ctx, fmt.Sprintf("writing template: %s", templatePath))
	defer span.End()
	var buffer bytes.Buffer

	name := path.Base(templatePath)

	parsedTemplate, templateErr := template.New(name).ParseFS(fs, templatePath)
	if templateErr != nil {
		return buffer, templateErr
	}
	if err := parsedTemplate.Execute(&buffer, data); err != nil {
		return buffer, err
	}
	return buffer, nil
}

func ConfirmDialog(messageFormat string, a ...any) bool {
	response := ""
	fmt.Printf(messageFormat, a...)
	_, err := fmt.Scan(&response)
	if err != nil {
		return false
	}
	return strings.EqualFold(response, "y")
}
