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

import "os/exec"

type Deb822Repo struct {
	Types      string
	URIs       string
	Suites     string
	Components string
}

func RunInContainer(mount string, args ...string) *exec.Cmd {
	prepend := append([]string{"-D", mount}, args...)
	return exec.Command("systemd-nspawn", prepend...)
}

func Packages() error {
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

	if err := RunInContainer(mount, basePackages...).Run(); err != nil {
		return err
	}

	return nil
}
