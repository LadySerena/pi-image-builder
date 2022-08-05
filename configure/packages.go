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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
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
	mount = "./mnt"
)

type ErrStatusCode struct {
	expectedCode int
	statusCode   int
}

func NewErrStatusCode(expectedCode int, statusCode int) *ErrStatusCode {
	return &ErrStatusCode{expectedCode: expectedCode, statusCode: statusCode}
}

type KubernetesDownload struct {
	name    string
	version string
	arch    string
}

type KubernetesSystemd struct {
	KubeletPath string
}

func NewKubernetesDownload(name string, version string, arch string) *KubernetesDownload {
	return &KubernetesDownload{name: name, version: version, arch: arch}
}

func (d KubernetesDownload) URL() string {
	return fmt.Sprintf("https://storage.googleapis.com/kubernetes-release/release/%s/bin/linux/%s/%s", d.version, d.arch, d.name)
}

func (e ErrStatusCode) Error() string {
	return fmt.Sprintf("expected http code: %d, got %d instead", e.expectedCode, e.statusCode)
}

// Deprecated: Write this method is brittle and there is a better go template solution
func (r Deb822Repo) Write(w io.Writer) (int, error) {
	repoString := fmt.Sprintf(repoString, r.Types, r.URIs, r.Suites, r.Components, r.Arch)
	return w.Write([]byte(repoString))
}

func RunCommandWithOutput(cmd *exec.Cmd) error {

	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("non zero exit code exit code: %v, output: %s", err, string(output))
	}

	return nil
}

func NspawnCommand(mount string, args ...string) *exec.Cmd {
	prepend := append([]string{"-D", mount}, args...)
	command := exec.Command("systemd-nspawn", prepend...)
	return command
}

func Packages(fs afero.Fs) error {
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

	if err := RunCommandWithOutput(NspawnCommand(mount, "apt-get", "update")); err != nil {
		return err
	}

	if err := RunCommandWithOutput(NspawnCommand(mount, append([]string{"apt-get", "install", "-y"}, basePackages...)...)); err != nil {
		return err
	}

	client := http.Client{Timeout: time.Minute * 2}

	response, dockerKeyErr := client.Get("https://download.docker.com/linux/ubuntu/gpg")
	if dockerKeyErr != nil {
		return dockerKeyErr
	}
	defer utility.WrappedClose(response.Body)

	if err := IdempotentWrite(fs, response.Body, "/etc/apt/trusted.gpg.d/docker.asc", 0644); err != nil {
		return err
	}

	dockerRepo := Deb822Repo{
		Types:      "deb",
		URIs:       "https://download.docker.com/linux/ubuntu",
		Suites:     "focal",
		Components: "stable",
		Arch:       "arm64",
	}

	dockerSources, dockerErr := RenderTemplate(configFiles, "files/Deb822.template", dockerRepo)
	if dockerErr != nil {
		return dockerErr
	}

	if err := IdempotentWrite(fs, &dockerSources, "/etc/apt/sources.list.d/docker.sources", 0644); err != nil {
		return err
	}

	if err := RunCommandWithOutput(NspawnCommand(mount, "apt-get", "update")); err != nil {
		return err
	}

	if err := RunCommandWithOutput(NspawnCommand(mount, "apt-get", "install", "-y", "containerd.io")); err != nil {
		return err
	}

	containerdConfig, containerdErr := configFiles.ReadFile("files/containerd-config.toml")
	if containerdErr != nil {
		return containerdErr
	}

	if err := afero.WriteFile(fs, "/etc/containerd/config.toml", containerdConfig, 0644); err != nil {
		return err
	}

	return nil
}

func InstallKubernetes(fs afero.Fs, kubernetesVersion string, criCtlVersion string, cniVersion string) error {

	const arch = "arm64"
	const cniDir = "/opt/cni/bin/"
	const downloadDir = "/usr/local/bin/"

	client := http.Client{Timeout: time.Minute * 5}

	if err := fs.MkdirAll(cniDir, 0775); err != nil {
		return err
	}

	if err := fs.MkdirAll(downloadDir, 0755); err != nil {
		return err
	}

	cniDownload, cniDownloadErr := client.Get(fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-%s-%s.tgz", cniVersion, arch, cniVersion))
	if cniDownloadErr != nil {
		return cniDownloadErr
	}
	if cniDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, cniDownload.StatusCode)
	}
	defer utility.WrappedClose(cniDownload.Body)

	cniFs := afero.NewBasePathFs(fs, cniDir)
	if err := ExtractTarGz(cniFs, cniDownload.Body); err != nil {
		return err
	}

	criCtlDownload, criCtlDownloadErr := client.Get(fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-%s-linux-%s.tar.gz", criCtlVersion, criCtlVersion, arch))
	if criCtlDownloadErr != nil {
		return criCtlDownloadErr
	}
	if criCtlDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, criCtlDownload.StatusCode)
	}
	defer utility.WrappedClose(criCtlDownload.Body)

	kubernetesFs := afero.NewBasePathFs(fs, downloadDir)

	if err := ExtractTarGz(kubernetesFs, criCtlDownload.Body); err != nil {
		return err
	}

	kubeadmDownload, kubeadmErr := client.Get(NewKubernetesDownload("kubeadm", kubernetesVersion, arch).URL())
	if kubeadmErr != nil {
		return kubeadmErr
	}
	if kubeadmDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubeadmDownload.StatusCode)
	}
	defer utility.WrappedClose(kubeadmDownload.Body)

	if err := IdempotentWrite(kubernetesFs, kubeadmDownload.Body, "kubeadm", 0755); err != nil {
		return err
	}

	kubeletDownload, kubeletErr := client.Get(NewKubernetesDownload("kubelet", kubernetesVersion, arch).URL())
	if kubeletErr != nil {
		return kubeletErr
	}
	if kubeletDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubeletDownload.StatusCode)
	}
	defer utility.WrappedClose(kubeletDownload.Body)

	if err := IdempotentWrite(kubernetesFs, kubeletDownload.Body, "kubelet", 0755); err != nil {
		return err
	}

	kubectlDownload, kubectlErr := client.Get(NewKubernetesDownload("kubectl", kubernetesVersion, arch).URL())
	if kubectlErr != nil {
		return kubectlErr
	}
	if kubectlDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubectlDownload.StatusCode)
	}
	defer utility.WrappedClose(kubectlDownload.Body)

	if err := IdempotentWrite(kubernetesFs, kubectlDownload.Body, "kubectl", 0755); err != nil {
		return err
	}

	kubeletPath := KubernetesSystemd{KubeletPath: path.Join(downloadDir, "kubelet")}

	systemdUnit, systemdErr := RenderTemplate(configFiles, "files/kubelet.service.template", kubeletPath)
	if systemdErr != nil {
		return systemdErr
	}

	if err := IdempotentWrite(fs, &systemdUnit, "/etc/systemd/system/kubelet.service", 0644); err != nil {
		return err
	}

	if err := fs.MkdirAll("/etc/systemd/system/kubelet.service.d", 0755); err != nil {
		return err
	}

	dropIn, dropInErr := RenderTemplate(configFiles, "files/kubeadm-drop-in.template", kubeletPath)
	if dropInErr != nil {
		return dropInErr
	}

	if err := IdempotentWrite(fs, &dropIn, "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf", 0644); err != nil {
		return err
	}

	if err := RunCommandWithOutput(NspawnCommand(mount, "systemctl", "enable", "kubelet")); err != nil {
		return err
	}

	return nil
}

func ConfigureCloudInit(fs afero.Fs) error {

	cloudInitDropInDir := "/etc/cloud/cloud.cfg.d/"
	user, userErr := configFiles.Open("files/06_user.cfg.yml")
	if userErr != nil {
		return userErr
	}

	if err := IdempotentWrite(fs, user, path.Join(cloudInitDropInDir, "06_user.cfg"), 0644); err != nil {
		return err
	}

	network, networkErr := configFiles.Open("files/07_network.cfg.yml")
	if networkErr != nil {
		return networkErr
	}

	if err := IdempotentWrite(fs, network, path.Join(cloudInitDropInDir, "07_network.cfg"), 0644); err != nil {
		return err
	}

	promisc, promiscErr := configFiles.Open("files/promisc.sh")
	if promiscErr != nil {
		return promiscErr
	}

	if err := IdempotentWrite(fs, promisc, "/etc/networkd-dispatcher/routable.d/promisc.sh", 0644); err != nil {
		return err
	}

	return nil
}

func ExtractTarGz(fs afero.Fs, r io.Reader) error {

	uncompressedStream, gzipErr := gzip.NewReader(r)
	if gzipErr != nil {
		return gzipErr
	}
	defer utility.WrappedClose(uncompressedStream)
	tarReader := tar.NewReader(uncompressedStream)
	for {
		header, headerErr := tarReader.Next()
		if headerErr == io.EOF {
			break
		}
		if headerErr != nil {
			return headerErr
		}
		if header.FileInfo().IsDir() {
			continue
		}
		file, fileErr := fs.OpenFile(header.Name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, header.FileInfo().Mode())
		if fileErr != nil {
			return fileErr
		}
		if _, err := io.Copy(file, tarReader); err != nil { //nolint:gosec
			return err
		}

	}
	return nil
}

func IdempotentWrite(fs afero.Fs, reader io.Reader, path string, mode os.FileMode) error {

	incomingData, readErr := io.ReadAll(reader)
	if readErr != nil {
		return readErr
	}
	file, fileOpenErr := fs.OpenFile(path, os.O_CREATE|os.O_RDWR, mode)
	if fileOpenErr != nil && !errors.Is(fileOpenErr, os.ErrExist) {
		return fileOpenErr
	}
	defer utility.WrappedClose(file)

	currentData, currentErr := io.ReadAll(file)
	if currentErr != nil {
		return currentErr
	}

	if bytes.Equal(incomingData, currentData) {
		return nil
	}

	if _, err := file.Write(incomingData); err != nil {
		return err
	}

	return nil
}
