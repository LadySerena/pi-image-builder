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
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path"
	"time"

	"github.com/LadySerena/pi-image-builder/telemetry"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

type Deb822Repo struct {
	Types      string
	URIs       string
	Suites     string
	Components string
	Arch       string
}

const (
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

func NspawnCommand(ctx context.Context, mount string, timeout time.Duration, args ...string) (*exec.Cmd, context.CancelFunc) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	prepend := append([]string{"--setenv=DEBIAN_FRONTEND=noninteractive", "-D", mount}, args...)
	command := exec.CommandContext(ctx, "systemd-nspawn", prepend...)
	return command, cancel
}

func Packages(ctx context.Context, fs afero.Fs) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "install packages")
	defer span.End()

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
		"apt-transport-https",
		"nftables",
		"conntrack",
		"lvm2",
		"bash",
		"util-linux", // findmnt blkid and lsblk for longhorn
		"grep",
		"open-iscsi",
	}

	update, updateCancel := NspawnCommand(ctx, mount, 5*time.Minute, "apt-get", "update")
	if err := utility.RunCommandWithOutput(ctx, update, updateCancel); err != nil {
		return err
	}

	uninstall, uninstallCancel := NspawnCommand(ctx, mount, 20*time.Minute, "apt-get", "purge", "-y", "snapd")
	if err := utility.RunCommandWithOutput(ctx, uninstall, uninstallCancel); err != nil {
		return err
	}

	install, installCancel := NspawnCommand(ctx, mount, 20*time.Minute, append([]string{"apt-get", "install", "--no-install-recommends", "-y"}, basePackages...)...)
	if err := utility.RunCommandWithOutput(ctx, install, installCancel); err != nil {
		return err
	}

	response, dockerKeyErr := otelhttp.Get(ctx, "https://download.docker.com/linux/ubuntu/gpg")
	if dockerKeyErr != nil {
		return dockerKeyErr
	}
	defer utility.WrappedClose(response.Body)

	if err := IdempotentWrite(ctx, fs, response.Body, "/etc/apt/trusted.gpg.d/docker.asc", 0644); err != nil {
		return err
	}

	dockerRepo := Deb822Repo{
		Types:      "deb",
		URIs:       "https://download.docker.com/linux/ubuntu",
		Suites:     "focal",
		Components: "stable",
		Arch:       "arm64",
	}

	dockerSources, dockerErr := utility.RenderTemplate(ctx, configFiles, "files/Deb822.template", dockerRepo)
	if dockerErr != nil {
		return dockerErr
	}

	if err := IdempotentWrite(ctx, fs, &dockerSources, "/etc/apt/sources.list.d/docker.sources", 0644); err != nil {
		return err
	}

	preDockerUpdate, dockerUpdateCancel := NspawnCommand(ctx, mount, 5*time.Minute, "apt-get", "update")

	if err := utility.RunCommandWithOutput(ctx, preDockerUpdate, dockerUpdateCancel); err != nil {
		return err
	}
	// todo feature flag this
	upgrade, upgradeCancel := NspawnCommand(ctx, mount, 20*time.Minute, "apt-get", "upgrade", "-y")

	if err := utility.RunCommandWithOutput(ctx, upgrade, upgradeCancel); err != nil {
		return err
	}

	dockerInstall, dockerCancel := NspawnCommand(ctx, mount, 20*time.Minute, "apt-get", "install", "-y", "containerd.io")

	if err := utility.RunCommandWithOutput(ctx, dockerInstall, dockerCancel); err != nil {
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

func InstallKubernetes(ctx context.Context, fs afero.Fs, kubernetesVersion string, criCtlVersion string, cniVersion string) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "install kubernetes")
	defer span.End()

	const arch = "arm64"
	const cniDir = "/opt/cni/bin/"
	const downloadDir = "/usr/local/bin/"

	if err := fs.MkdirAll(cniDir, 0775); err != nil {
		return err
	}

	if err := fs.MkdirAll(downloadDir, 0755); err != nil {
		return err
	}

	cniDownload, cniDownloadErr := otelhttp.Get(ctx, fmt.Sprintf("https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-linux-%s-%s.tgz", cniVersion, arch, cniVersion))
	if cniDownloadErr != nil {
		return cniDownloadErr
	}
	if cniDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, cniDownload.StatusCode)
	}
	defer cniDownload.Body.Close()

	cniFs := afero.NewBasePathFs(fs, cniDir)
	if err := ExtractTarGz(ctx, cniFs, cniDownload.Body); err != nil {
		return err
	}

	criCtlDownload, criCtlDownloadErr := otelhttp.Get(ctx, fmt.Sprintf("https://github.com/kubernetes-sigs/cri-tools/releases/download/%s/crictl-%s-linux-%s.tar.gz", criCtlVersion, criCtlVersion, arch))
	if criCtlDownloadErr != nil {
		return criCtlDownloadErr
	}
	if criCtlDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, criCtlDownload.StatusCode)
	}
	defer cniDownload.Body.Close()

	kubernetesFs := afero.NewBasePathFs(fs, downloadDir)

	if err := ExtractTarGz(ctx, kubernetesFs, criCtlDownload.Body); err != nil {
		return err
	}

	kubeadmDownload, kubeadmErr := otelhttp.Get(ctx, NewKubernetesDownload("kubeadm", kubernetesVersion, arch).URL())
	if kubeadmErr != nil {
		return kubeadmErr
	}
	if kubeadmDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubeadmDownload.StatusCode)
	}
	defer kubeadmDownload.Body.Close()

	if err := IdempotentWrite(ctx, kubernetesFs, kubeadmDownload.Body, "kubeadm", 0755); err != nil {
		return err
	}

	kubeletDownload, kubeletErr := otelhttp.Get(ctx, NewKubernetesDownload("kubelet", kubernetesVersion, arch).URL())
	if kubeletErr != nil {
		return kubeletErr
	}
	if kubeletDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubeletDownload.StatusCode)
	}
	defer kubeletDownload.Body.Close()

	if err := IdempotentWrite(ctx, kubernetesFs, kubeletDownload.Body, "kubelet", 0755); err != nil {
		return err
	}

	kubectlDownload, kubectlErr := otelhttp.Get(ctx, NewKubernetesDownload("kubectl", kubernetesVersion, arch).URL())
	if kubectlErr != nil {
		return kubectlErr
	}
	if kubectlDownload.StatusCode != http.StatusOK {
		return NewErrStatusCode(http.StatusOK, kubectlDownload.StatusCode)
	}
	defer kubectlDownload.Body.Close()

	if err := IdempotentWrite(ctx, kubernetesFs, kubectlDownload.Body, "kubectl", 0755); err != nil {
		return err
	}

	kubeletPath := KubernetesSystemd{KubeletPath: path.Join(downloadDir, "kubelet")}

	systemdUnit, systemdErr := utility.RenderTemplate(ctx, configFiles, "files/kubelet.service.template", kubeletPath)
	if systemdErr != nil {
		return systemdErr
	}

	if err := IdempotentWrite(ctx, fs, &systemdUnit, "/etc/systemd/system/kubelet.service", 0644); err != nil {
		return err
	}

	if err := fs.MkdirAll("/etc/systemd/system/kubelet.service.d", 0755); err != nil {
		return err
	}

	dropIn, dropInErr := utility.RenderTemplate(ctx, configFiles, "files/kubeadm-drop-in.template", kubeletPath)
	if dropInErr != nil {
		return dropInErr
	}

	if err := IdempotentWrite(ctx, fs, &dropIn, "/etc/systemd/system/kubelet.service.d/10-kubeadm.conf", 0644); err != nil {
		return err
	}

	enableKubelet, kubeletCancel := NspawnCommand(ctx, mount, 5*time.Minute, "systemctl", "enable", "kubelet")

	if err := utility.RunCommandWithOutput(ctx, enableKubelet, kubeletCancel); err != nil {
		return err
	}

	return nil
}

func CloudInit(ctx context.Context, fs afero.Fs) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "configure cloudinit")
	defer span.End()

	cloudInitDropInDir := "/etc/cloud/cloud.cfg.d/"
	user, userErr := configFiles.Open("files/06_user.cfg.yml")
	if userErr != nil {
		return userErr
	}

	if err := IdempotentWrite(ctx, fs, user, path.Join(cloudInitDropInDir, "06_user.cfg"), 0644); err != nil {
		return err
	}

	network, networkErr := configFiles.Open("files/07_network.cfg.yml")
	if networkErr != nil {
		return networkErr
	}

	if err := IdempotentWrite(ctx, fs, network, path.Join(cloudInitDropInDir, "07_network.cfg"), 0644); err != nil {
		return err
	}

	promisc, promiscErr := configFiles.Open("files/promisc.sh")
	if promiscErr != nil {
		return promiscErr
	}

	if err := IdempotentWrite(ctx, fs, promisc, "/etc/networkd-dispatcher/routable.d/promisc.sh", 0644); err != nil {
		return err
	}

	return nil
}

func Fstab(ctx context.Context, fs afero.Fs) error {
	_, span := telemetry.GetTracer().Start(ctx, "configure fstab entries")
	defer span.End()

	fstab, fstabErr := configFiles.ReadFile("files/fstab")
	if fstabErr != nil {
		return fstabErr
	}

	if dirErr := fs.MkdirAll("/var/lib/longhorn", 0750); dirErr != nil {
		return dirErr
	}

	if dirErr := fs.MkdirAll("/var/lib/containerd", 0750); dirErr != nil {
		return dirErr
	}

	return afero.WriteFile(fs, "/etc/fstab", fstab, 0644)
}

func ExtractTarGz(ctx context.Context, fs afero.Fs, r io.Reader) error {

	_, span := telemetry.GetTracer().Start(ctx, "Extract tar.gz")
	defer span.End()

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
		span.AddEvent(fmt.Sprintf("writing file: %s", header.Name))
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

func IdempotentWrite(ctx context.Context, fs afero.Fs, reader io.Reader, path string, mode os.FileMode) error {

	_, span := telemetry.GetTracer().Start(ctx, fmt.Sprintf("writing: %s", path))
	defer span.End()

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
