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
	"embed"
)

//go:embed files/*
var configFiles embed.FS

const (
	commandLine = "dwc_otg.lpm_enable=0 console=serial0,115200 console=tty1 root=/dev/rootvg/rootlv rootfstype=ext4 elevator=deadline rootwait fixrtc quiet splash cgroup_enable=memory swapaccount=1 cgroup_memory=1 cgroup_enable=cpuset"
	postInvoke  = `DPkg::Post-Invoke {"/bin/bash /boot/auto_decompress_kernel"; };`

	commandLinePath = "/boot/firmware/cmdline.txt"
)
