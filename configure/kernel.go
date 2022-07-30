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
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/spf13/afero"
)

const (
	commandLine    = "dwc_otg.lpm_enable=0 console=serial0,115200 console=tty1 root=LABEL=writable rootfstype=ext4 elevator=deadline rootwait fixrtc quiet splash cgroup_enable=memory swapaccount=1 cgroup_memory=1 cgroup_enable=cpuset"
	firmwareConfig = `[pi4]
max_framebuffers=2
dtoverlay=vc4-fkms-v3d
boot_delay
kernel=vmlinux
initramfs initrd.img followkernel
`
	decompressKernel = `#!/bin/bash -e
# auto_decompress_kernel script
BTPATH=/boot/firmware
CKPATH=$BTPATH/vmlinuz
DKPATH=$BTPATH/vmlinux
# Check if compression needs to be done.
if [ -e $BTPATH/check.md5 ]; then
   if md5sum --status --ignore-missing -c $BTPATH/check.md5; then
      echo -e "\e[32mFiles have not changed, Decompression not needed\e[0m"
      exit 0
   else
      echo -e "\e[31mHash failed, kernel will be compressed\e[0m"
   fi
fi
# Backup the old decompressed kernel
mv $DKPATH $DKPATH.bak
if [ ! $? == 0 ]; then
   echo -e "\e[31mDECOMPRESSED KERNEL BACKUP FAILED!\e[0m"
   exit 1
else
   echo -e "\e[32mDecompressed kernel backup was successful\e[0m"
fi
# Decompress the new kernel
echo "Decompressing kernel: "$CKPATH".............."
zcat -qf $CKPATH > $DKPATH
if [ ! $? == 0 ]; then
   echo -e "\e[31mKERNEL FAILED TO DECOMPRESS!\e[0m"
   exit 1
else
   echo -e "\e[32mKernel Decompressed Succesfully\e[0m"
fi
# Hash the new kernel for checking
md5sum $CKPATH $DKPATH > $BTPATH/check.md5
if [ ! $? == 0 ]; then
   echo -e "\e[31mMD5 GENERATION FAILED!\e[0m"
else
   echo -e "\e[32mMD5 generated Succesfully\e[0m"
fi
exit 0
`
	postInvoke = `DPkg::Post-Invoke {"/bin/bash /boot/auto_decompress_kernel"; };`

	commandLinePath = "/boot/firmware/cmdline.txt"
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

func KernelSettings(fs afero.Fs) error {

	if err := afero.WriteFile(fs, commandLinePath, []byte(commandLine), 0755); err != nil {
		return err
	}

	if err := afero.WriteFile(fs, "/boot/auto_decompress_kernel", []byte(decompressKernel), 0544); err != nil {
		return err
	}

	if err := afero.WriteFile(fs, "/boot/firmware/usercfg.txt", []byte(firmwareConfig), 0755); err != nil {
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

	if err := afero.WriteFile(fs, "/etc/apt/apt.conf.d/999_decompress_rpi_kernel", []byte(postInvoke), 0644); err != nil {
		return err
	}

	return nil
}

func KernelModules(fs afero.Fs) error {

	modules := strings.Join([]string{"br_netfilter", "overlay"}, "\n")
	kubernetesSysctlPath := "/etc/modules-load.d/k8s.conf"
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
