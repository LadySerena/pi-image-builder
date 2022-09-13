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
	"log"

	"github.com/LadySerena/pi-image-builder/partition"
	"github.com/LadySerena/pi-image-builder/utility"
	flag "github.com/spf13/pflag"
)

func main() {
	//todo local or gsutil path for image
	//todo validate block device
	//todo blocklist for devices (prevent juggling chainsaws)
	//todo flag to declare how to slice up the remaining logical partition
	//todo actually might be better to have a config file for spacing 🤔?

	imageName := flag.StringP("image", "i", "", "specify your desired image")
	outputDevice := flag.StringP("device", "d", "", "specify which target device to flash the image")

	flag.Parse()

	if *imageName == "" {
		panic("you must specify a valid disk image")
	}

	if *outputDevice == "" {
		panic("you must specify a valid block device")
	}

	//ctx := context.TODO()
	//
	//gcsClient, gcsErr := storage.NewClient(ctx)
	//if gcsErr != nil {
	//	log.Panicf("error creating cloud storage client: %v", gcsErr)
	//}
	//
	//localFs := afero.NewOsFs()

	answer := utility.ConfirmDialog("are you sure you want to flash the image to %s: [Y/n]: ", *outputDevice)
	if !answer {
		fmt.Println("nope")
		return
	}

	if err := partition.CreateTable(*outputDevice); err != nil {
		log.Panicf("could not create partitions: %v", err)
	}
}
