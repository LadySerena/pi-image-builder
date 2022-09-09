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

package main

import (
	"fmt"

	"github.com/LadySerena/pi-image-builder/utility"
	flag "github.com/spf13/pflag"
)

func main() {
	//todo flag to determine which image I'm using
	//todo flag for block device to flash
	//todo flag to declare how to slice up the remaining logical partition
	//todo actually might be better to have a config file for spacing 🤔?

	imageName := flag.StringP("image", "i", "", "specify your desired image")
	outputDevice := flag.StringP("device", "d", "", "specify which target device to flash the image")

	flag.Parse()

	if *imageName == "" {
	}

	ifDevice := "/dev/foobar"
	answer := utility.ConfirmDialog("are you sure you want to flash the image to %s: [Y/n]: ", ifDevice)
	if !answer {
		fmt.Println("nope")
		return
	}
	fmt.Println("yep")
}
