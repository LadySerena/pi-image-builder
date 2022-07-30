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

package configure

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"time"

	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
)

type Deb822Repo struct {
	Types      string
	URIs       string
	Suites     string
	Components string
	Arch       string
}

const (
	repoString = `Types: %s
URIs: %s
Suites: %s
Components: %s
Architectures: %s
`
)

func (r Deb822Repo) Write(w io.Writer) (int, error) {
	repoString := fmt.Sprintf(repoString, r.Types, r.URIs, r.Suites, r.Components, r.Arch)
	return w.Write([]byte(repoString))
}

func RunInContainer(mount string, args ...string) *exec.Cmd {
	prepend := append([]string{"-D", mount}, args...)
	return exec.Command("systemd-nspawn", prepend...)
}

func Packages(fileSystem afero.Fs) error {
	mount := "./mnt"
	basePackages := []string{
		"openssh-server",
		"ca-certificates",
		"curl",
		"lsb-release",
		"wget",
		"gnupg",
		"sudo",
		"lm-sensors",
		"perl",
		"htop",
		"crudini",
		"bat",
		"apt-transport-https",
		"nftables",
		"conntrack",
	}

	if err := RunInContainer(mount, "apt-get", "update").Run(); err != nil {
		return err
	}

	if err := RunInContainer(mount, append([]string{"apt-get", "install", "-y"}, basePackages...)...).Run(); err != nil {
		return err
	}

	client := http.Client{Timeout: time.Minute * 2}

	response, dockerKeyErr := client.Get("https://download.docker.com/linux/ubuntu/gpg")
	if dockerKeyErr != nil {
		return dockerKeyErr
	}
	defer utility.WrappedClose(response.Body)

	dockerKeyFile, fileOpenErr := fileSystem.OpenFile("/etc/apt/trusted.gpg.d/docker.asc", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if fileOpenErr != nil {
		return fileOpenErr
	}
	defer utility.WrappedClose(dockerKeyFile)

	if _, err := io.Copy(dockerKeyFile, response.Body); err != nil {
		return err
	}

	dockerRepo := Deb822Repo{
		Types:      "deb",
		URIs:       "https://download.docker.com/linux/ubuntu",
		Suites:     "focal",
		Components: "stable",
		Arch:       "arm64",
	}

	dockerRepoFile, dockerOpenErr := fileSystem.OpenFile("/etc/apt/sources.list.d/docker.sources", os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)
	if dockerOpenErr != nil {
		return dockerOpenErr
	}

	defer utility.WrappedClose(dockerRepoFile)

	if _, err := dockerRepo.Write(dockerRepoFile); err != nil {
		return err
	}

	return nil
}
