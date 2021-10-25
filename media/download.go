/*
 * Copyright (c) 2021 Serena Tiede
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

package media

import (
	"bytes"
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"time"

	"golang.org/x/sync/errgroup"
)

func DownloadAndVerifyMedia() error {

	client := http.Client{
		Timeout: time.Minute * 5,
	}

	mediaName := "ArchLinuxARM-rpi-aarch64-latest.tar.gz"
	checksumName := fmt.Sprintf("%s.md5", mediaName)

	group := new(errgroup.Group)
	group.Go(func() error {
		return DownloadFile(&client, mediaName, fmt.Sprintf("http://os.archlinuxarm.org/os/%s", mediaName))
	})
	group.Go(func() error {
		return DownloadFile(&client, checksumName, fmt.Sprintf("http://os.archlinuxarm.org/os/%s", checksumName))
	})
	if waitErr := group.Wait(); waitErr != nil {
		return waitErr
	}
	media, mediaErr := os.Open(mediaName)
	if mediaErr != nil {
		return mediaErr
	}
	hash := md5.New()
	// media is read and hashed because the hash type implements the io.writer interface
	if _, copyErr := io.Copy(hash, media); copyErr != nil {
		return copyErr
	}
	foo := hex.EncodeToString(hash.Sum(nil))

	checksum, readErr := os.ReadFile(checksumName)
	if readErr != nil {
		return readErr
	}
	foo2 := bytes.Split(checksum, []byte(" "))

	if foo != string(foo2[0]) {
		return errors.New("checksums do not match")
	}
	return nil
}

func DownloadFile(client *http.Client, fileName string, url string) error {
	media, mediaErr := os.Create(fileName)
	if mediaErr != nil {
		log.Fatalf("could not create file: %v", mediaErr)
	}
	defer func(media *os.File) {
		err := media.Close()
		if err != nil {
			log.Fatalf("could not close file properly: %v", err)
		}
	}(media)

	mediaResponse, mediaDownloadErr := client.Get(url)
	if mediaDownloadErr != nil {
		log.Fatalf("could not download file: %v", mediaDownloadErr)
	}
	defer func(Body io.ReadCloser) {
		err := Body.Close()
		if err != nil {
			log.Fatalf("could not close body properly: %v", err)
		}
	}(mediaResponse.Body)

	_, copyErr := io.Copy(media, mediaResponse.Body)
	if copyErr != nil {
		log.Fatalf("could not copy http body to filesystem: %v", copyErr)
	}
	return nil
}
