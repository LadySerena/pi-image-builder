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
	"context"
	"fmt"
	"log"
	"strings"

	"cloud.google.com/go/storage"
	"github.com/LadySerena/pi-image-builder/media"
	"github.com/LadySerena/pi-image-builder/partition"
	"github.com/LadySerena/pi-image-builder/utility"
	"github.com/klauspost/compress/zstd"
	"github.com/spf13/afero"
	flag "github.com/spf13/pflag"
)

func main() {
	// todo local or gsutil path for image

	const decompressedImageFileName = "image-to-be-flashed.img"

	imageName := flag.StringP("image", "i", "", "specify your desired image")
	outputDevice := flag.StringP("device", "d", "", "specify which target device to flash the image")

	flag.Parse()

	if *imageName == "" {
		panic("you must specify a valid disk image")
	}

	if *outputDevice == "" || !strings.Contains(*outputDevice, "/dev") {
		panic("you must specify a valid block device")
	}

	answer := utility.ConfirmDialog("are you sure you want to flash the image to %s: [Y/n]: ", *outputDevice)
	if !answer {
		fmt.Println("nope")
		return
	}

	ctx := context.TODO()

	localFs := afero.NewOsFs()
	downloadExists, statErr := afero.Exists(localFs, *imageName)
	if statErr != nil {
		log.Panicf("could not verify file: %v", statErr)
	}

	// if image is downloaded skip downloading it
	if !downloadExists {
		gcsClient, gcsErr := storage.NewClient(ctx)
		if gcsErr != nil {
			log.Panicf("error creating cloud storage client: %v", gcsErr)
		}

		reader, readerCreateErr := gcsClient.Bucket(utility.BucketName).Object(*imageName).NewReader(ctx)
		if readerCreateErr != nil {
			log.Panicf("error creating reader for image: %s error: %v", *imageName, readerCreateErr)
		}
		defer utility.WrappedClose(reader)

		if writeErr := afero.WriteReader(localFs, *imageName, reader); writeErr != nil {
			log.Panicf("error writing file: %v", writeErr)
		}
	}

	decompressExists, decompressStatErr := afero.Exists(localFs, decompressedImageFileName)
	if decompressStatErr != nil {
		log.Panic(decompressStatErr)
	}

	// if image is decompressed then skip it
	if !decompressExists {
		image, openErr := localFs.Open(*imageName)
		if openErr != nil {
			log.Panicf("could not open image file: %v", openErr)
		}
		defer utility.WrappedClose(image)
		decompress, decompressErr := zstd.NewReader(image)
		if decompressErr != nil {
			log.Panicf("could not decompress image: %v", decompressErr)
		}
		defer decompress.Close()

		decompressedOutput, outputErr := localFs.Create(decompressedImageFileName)
		if outputErr != nil {
			log.Panicf("could not open file handle for decompressed file: %v", outputErr)
		}
		defer utility.WrappedClose(decompressedOutput)

		if _, err := decompress.WriteTo(decompressedOutput); err != nil {
			log.Panicf("error during image decompression: %v", err)
		}
	}

	if err := partition.CreateTable(*outputDevice); err != nil {
		log.Panicf("could not create partitions: %v", err)
	}

	if err := partition.CreateLogicalVolumes(*outputDevice); err != nil {
		log.Panicf("could not create logical volumes: %v", err)
	}

	if err := partition.CreateFileSystems(*outputDevice); err != nil {
		log.Panicf("could not create filesystems: %v", err)
	}

	entry, loopErr := media.MountImageToDevice(ctx, decompressedImageFileName)
	if loopErr != nil {
		log.Panicf("could not create loop device for image: %v", loopErr)
	}

	if err := media.AttachToMountPoint(ctx, localFs, entry, false); err != nil {
		log.Panicf("could not attach loop device: %s to mount points: %v", entry.Name, err)
	}

	if err := media.MountMedia(ctx, localFs, *outputDevice); err != nil {
		log.Panicf("could not mount media: %v", err)
	}

	if err := media.Flash(ctx, *outputDevice, entry); err != nil {
		log.Panicf("could not rsync data from image to media: %v", err)
	}

	// todo add cleanup code
}
