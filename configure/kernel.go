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
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/LadySerena/pi-image-builder/telemetry"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
)

type Sysctl map[string]string

func (s Sysctl) String() string {

	var returnValue []string

	for key, value := range s {
		returnValue = append(returnValue, fmt.Sprintf("%s = %s", key, value))
	}

	return strings.Join(returnValue, "\n")
}

func (s Sysctl) Write(writer io.Writer) (int, error) {
	data := s.String()
	return writer.Write([]byte(data))
}

func KernelSettings(ctx context.Context, fs afero.Fs) error {

	ctx, span := telemetry.GetTracer().Start(ctx, "configure kernel")
	defer span.End()

	decompressKernel, decompressErr := configFiles.Open("files/decompressKernel.bash")
	if decompressErr != nil {
		return decompressErr
	}

	defer utility.WrappedClose(decompressKernel)

	commandLineHandle, commandLineOpenErr := fs.OpenFile(commandLinePath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0755)
	if commandLineOpenErr != nil {
		return commandLineOpenErr
	}
	defer utility.WrappedClose(commandLineHandle)

	if _, err := commandLineHandle.WriteString(commandLine); err != nil {
		return err
	}

	if err := IdempotentWrite(ctx, fs, decompressKernel, "/boot/auto_decompress_kernel", 0544); err != nil {
		return err
	}

	firmwareConfigFile, firmwareConfigErr := configFiles.Open("files/firmwareConfig")
	if firmwareConfigErr != nil {
		return firmwareConfigErr
	}
	defer utility.WrappedClose(firmwareConfigFile)

	if err := IdempotentWrite(ctx, fs, firmwareConfigFile, "/boot/firmware/usercfg.txt", 0755); err != nil {
		return err
	}

	compressedKernelImage, fsOpenErr := fs.Open("/boot/firmware/vmlinuz")
	if fsOpenErr != nil {
		return fsOpenErr
	}
	defer utility.WrappedClose(compressedKernelImage)

	decompressedKernelImage, openErr := fs.OpenFile("/boot/firmware/vmlinux", os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if openErr != nil {
		return openErr
	}
	defer utility.WrappedClose(decompressedKernelImage)

	reader, readerErr := gzip.NewReader(compressedKernelImage)
	if readerErr != nil {
		return readerErr
	}
	defer utility.WrappedClose(reader)

	buffer, decompressErr := io.ReadAll(reader)
	if decompressErr != nil {
		return decompressErr
	}

	_, writeErr := decompressedKernelImage.Write(buffer)
	if writeErr != nil {
		return writeErr
	}

	if err := IdempotentWrite(ctx, fs, bytes.NewBufferString(postInvoke), "/etc/apt/apt.conf.d/999_decompress_rpi_kernel", 0644); err != nil {
		return err
	}

	return nil
}

func KernelModules(ctx context.Context, fs afero.Fs) error {

	_, span := telemetry.GetTracer().Start(ctx, "configuring kernel modules")
	defer span.End()

	modules := strings.Join([]string{"br_netfilter", "overlay"}, "\n")
	kubernetesSysctlPath := "/etc/sysctl.d/10-kubernetes.conf"
	ciliumSysctlPath := "/etc/sysctl.d/99-override_cilium_rp_filter.conf"

	kubernetesSysctls := Sysctl{
		"net.bridge.bridge-nf-call-ip6tables": "1",
		"net.bridge.bridge-nf-call-iptables":  "1",
		"net.ipv4.ip_forward":                 "1",
	}

	ciliumSysctls := Sysctl{
		"net.ipv4.conf.lxc*.rp_filter":    "0",
		"net.ipv4.conf.all.rp_filter":     "0",
		"net.ipv4.conf.default.rp_filter": "0",
	}

	if err := afero.WriteFile(fs, "/etc/modules-load.d/k8s.conf", []byte(modules), 0644); err != nil {
		return err
	}

	kubernetesFile, kubernetesErr := fs.OpenFile(kubernetesSysctlPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if kubernetesErr != nil {
		return kubernetesErr
	}
	defer utility.WrappedClose(kubernetesFile)

	ciliumFile, ciliumErr := fs.OpenFile(ciliumSysctlPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if ciliumErr != nil {
		return ciliumErr
	}
	defer utility.WrappedClose(ciliumFile)

	if _, err := kubernetesSysctls.Write(kubernetesFile); err != nil {
		return err
	}

	if _, err := ciliumSysctls.Write(ciliumFile); err != nil {
		return err
	}

	return nil
}
