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

	containerdConfig = `version = 2
root = "/var/lib/containerd"
state = "/run/containerd"
plugin_dir = ""
disabled_plugins = []
required_plugins = []
oom_score = 0
[plugins]
[plugins."io.containerd.grpc.v1.cri".containerd]
snapshotter = "overlayfs"
default_runtime_name = "runc"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc]
runtime_type = "io.containerd.runc.v2"
[plugins."io.containerd.grpc.v1.cri".containerd.runtimes.runc.options]
SystemdCgroup = true
`
)
